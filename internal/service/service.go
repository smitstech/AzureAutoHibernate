//go:build windows

package service

import (
	"context"
	"sync"
	"time"

	"github.com/smitstech/AzureAutoHibernate/internal/appinfo"
	"github.com/smitstech/AzureAutoHibernate/internal/azure"
	"github.com/smitstech/AzureAutoHibernate/internal/config"
	"github.com/smitstech/AzureAutoHibernate/internal/logger"
	"github.com/smitstech/AzureAutoHibernate/internal/monitor"
	"github.com/smitstech/AzureAutoHibernate/internal/updater"
	"github.com/smitstech/AzureAutoHibernate/internal/version"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
)

const (
	// notificationThrottleDuration is the minimum time between warning notifications
	notificationThrottleDuration = 30 * time.Second
	// warningCheckInterval is the polling interval when in warning mode
	warningCheckInterval = 5 * time.Second
	// minCheckInterval is the minimum interval for idle state checks
	minCheckInterval = 5 * time.Second
)

type AutoHibernateService struct {
	config               *config.Config
	idleMonitor          *monitor.IdleMonitor
	azureClient          *azure.AzureClient
	notifierManager      *NotifierManager
	logger               logger.Logger
	stopChan             chan struct{}
	stopOnce             sync.Once  // Ensures stopChan is only closed once
	lastNotificationTime time.Time
	resumeAt             *time.Time // Tracks when system resumed from hibernate/sleep
	updatePending        bool       // Flag to indicate an update is ready to apply
}

func NewAutoHibernateService(cfg *config.Config, vmMetadata *azure.VMMetadata, log logger.Logger) *AutoHibernateService {
	now := time.Now()

	// Create notifier manager (optional - will be nil if notifier executable not found)
	notifierManager, err := NewNotifierManager(log)
	if err != nil {
		log.Warningf(logger.EventSessionInfoWarning, "Failed to create notifier manager: %v - notifications will not be sent", err)
		notifierManager = nil
	}

	return &AutoHibernateService{
		config: cfg,
		idleMonitor: monitor.NewIdleMonitor(
			cfg.NoUsersIdleMinutes,
			cfg.AllDisconnectedIdleMinutes,
			cfg.InactiveUserIdleMinutes,
			cfg.InactiveUserWarningMinutes,
			cfg.MinimumUptimeMinutes,
		),
		azureClient: azure.NewAzureClient(
			vmMetadata.SubscriptionId,
			vmMetadata.ResourceGroup,
			vmMetadata.VMName,
		),
		notifierManager: notifierManager,
		logger:          log,
		stopChan:        make(chan struct{}),
		resumeAt:        &now, // Initialize to service start time
	}
}

func (s *AutoHibernateService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPowerEvent

	changes <- svc.Status{State: svc.StartPending}

	// Start the notifier manager (if available)
	if s.notifierManager != nil {
		if err := s.notifierManager.Start(); err != nil {
			s.logger.Errorf(logger.EventSessionMonitorError, "Failed to start notifier manager: %v", err)
			// Continue without notifications rather than failing
			s.notifierManager = nil
		}
	}

	// Start the monitoring loop
	go s.monitorLoop()

	// Start the update check loop if auto-update is enabled
	if s.config.AutoUpdate {
		go s.updateLoop()
	}

	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
	s.logger.Info(logger.EventServiceStart, "Service started and running")
	s.logger.Infof(logger.EventServiceStart, "Running version: %s", version.Version)

loop:
	for c := range r {
		switch c.Cmd {
		case svc.Interrogate:
			changes <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			s.logger.Info(logger.EventServiceStop, "Service stop requested")
			break loop
		case svc.PowerEvent:
			// Handle power management events
			s.handlePowerEvent(c.EventType)
			changes <- c.CurrentStatus
		default:
			s.logger.Warningf(logger.EventSessionInfoWarning, "Unexpected control request #%d", c)
		}
	}

	changes <- svc.Status{State: svc.StopPending}
	s.stopOnce.Do(func() {
		close(s.stopChan)
	})
	time.Sleep(2 * time.Second) // Give monitor loop and update loop time to exit

	// Stop the notifier manager (if running)
	if s.notifierManager != nil {
		s.notifierManager.Stop()
	}

	return
}

// handlePowerEvent handles Windows power management events
func (s *AutoHibernateService) handlePowerEvent(eventType uint32) {
	const (
		PBT_APMRESUMEAUTOMATIC = 18 // System resumed from suspend (automatic)
		PBT_APMRESUMESUSPEND   = 7  // System resumed from suspend (user-initiated)
	)

	switch eventType {
	case PBT_APMRESUMEAUTOMATIC:
		// System resumed from hibernation or sleep (automatic)
		now := time.Now()
		s.resumeAt = &now
		s.idleMonitor.SetResumeTime(now)
		s.logger.Infof(logger.EventServiceStart, "System resumed from hibernation/sleep (automatic) at %s", now.Format("15:04:05"))
	case PBT_APMRESUMESUSPEND:
		// System resumed from hibernation or sleep (user-initiated)
		now := time.Now()
		s.resumeAt = &now
		s.idleMonitor.SetResumeTime(now)
		s.logger.Infof(logger.EventServiceStart, "System resumed from hibernation/sleep (user-initiated) at %s", now.Format("15:04:05"))
	}
}

// calculateNextCheckTime determines when to check next based on current state
func (s *AutoHibernateService) calculateNextCheckTime(inWarningMode bool) time.Duration {
	// If in warning mode, check frequently for cancellation detection
	if inWarningMode {
		return warningCheckInterval
	}

	// Calculate default check interval from minimum configured threshold
	// This ensures responsive behavior even when no active conditions exist (e.g., after hibernation)
	minThreshold := s.config.NoUsersIdleMinutes
	if s.config.AllDisconnectedIdleMinutes > 0 && (minThreshold == 0 || s.config.AllDisconnectedIdleMinutes < minThreshold) {
		minThreshold = s.config.AllDisconnectedIdleMinutes
	}
	if s.config.InactiveUserIdleMinutes > 0 && (minThreshold == 0 || s.config.InactiveUserIdleMinutes < minThreshold) {
		minThreshold = s.config.InactiveUserIdleMinutes
	}

	// Use minimum threshold as default, or fall back to 5 minutes if all thresholds are 0
	defaultCheckInterval := 5 * time.Minute
	if minThreshold > 0 {
		defaultCheckInterval = time.Duration(minThreshold) * time.Minute
	}

	// Get time until next threshold could be exceeded
	timeUntil, err := s.idleMonitor.GetTimeUntilThresholds()
	if err != nil {
		s.logger.Debugf(logger.EventIdleCheckError, "Error calculating time until threshold: %v, using default interval", err)
		return defaultCheckInterval
	}

	// If no active conditions (timeUntil == 0), use default interval
	if timeUntil <= 0 {
		return defaultCheckInterval
	}

	// Ensure we never check less frequently than the minimum
	if timeUntil < minCheckInterval {
		return minCheckInterval
	}

	return timeUntil
}

func (s *AutoHibernateService) monitorLoop() {
	s.logger.Infof(logger.EventMonitoringStarted, "Monitor loop started with dynamic polling")
	s.logger.Infof(logger.EventMonitoringStarted, "Idle thresholds: NoUsers=%dm, AllDisconnected=%dm, InactiveUser=%dm, InactiveUserWarning=%dm",
		s.config.NoUsersIdleMinutes,
		s.config.AllDisconnectedIdleMinutes,
		s.config.InactiveUserIdleMinutes,
		s.config.InactiveUserWarningMinutes)

	inWarningMode := false

	for {
		// Protect against panics in the monitor loop
		func() {
			defer func() {
				if r := recover(); r != nil {
					s.logger.Errorf(logger.EventIdleCheckError, "Panic recovered in monitor loop: %v", r)
					// Add a small delay to prevent tight loops in case of persistent panics
					time.Sleep(5 * time.Second)
				}
			}()

			// Perform the check
			s.performMonitorCheck(&inWarningMode)
		}()

		// Calculate next check time dynamically
		// Note: If hibernation was triggered, the VM will hibernate and this service
		// will be suspended along with the OS. When the VM resumes, execution will
		// continue from here and monitoring will resume automatically.
		nextCheckDuration := s.calculateNextCheckTime(inWarningMode)
		s.logger.Debugf(logger.EventIdleCheckInfo, "Next check in %v", nextCheckDuration.Round(time.Second))

		// Sleep until next check
		select {
		case <-time.After(nextCheckDuration):
			// Continue to next iteration
		case <-s.stopChan:
			s.logger.Info(logger.EventServiceStop, "Monitor loop stopping")
			return
		}
	}
}

// performMonitorCheck executes a single monitor check iteration
func (s *AutoHibernateService) performMonitorCheck(inWarningMode *bool) {
	// Perform the check
	shouldWarn, isHibernating := s.checkAndHibernate()

	// Handle warning mode transitions
	if shouldWarn && !*inWarningMode {
		// Entering warning mode - switch to faster checks
		*inWarningMode = true
		s.logger.Debugf(logger.EventIdleCheckInfo, "Entering warning mode, increasing check frequency to 5s")
	} else if !shouldWarn && *inWarningMode {
		// Exiting warning mode due to user activity or hibernation
		if isHibernating {
			// VM is hibernating - just reset state, no notifications needed
			*inWarningMode = false
			s.lastNotificationTime = time.Time{} // Reset notification timer
			s.logger.Debugf(logger.EventIdleCheckInfo, "Exiting warning mode due to hibernation")
		} else {
			// User activity detected - send cancellation notification
			*inWarningMode = false
			s.lastNotificationTime = time.Time{} // Reset notification timer
			s.logger.Debugf(logger.EventIdleCheckInfo, "Exiting warning mode, returning to dynamic polling")

			if s.notifierManager != nil {
				// First, dismiss any active warning notification
				err := s.notifierManager.DismissWarning()
				if err != nil {
					s.logger.Debugf(logger.EventNotificationError, "Failed to dismiss warning notification: %v", err)
				} else {
					s.logger.Debugf(logger.EventHibernationWarningCancel, "Warning notification dismissed")
				}

				// Then send cancellation notification to user
				err = s.notifierManager.SendCancellation()
				if err != nil {
					s.logger.Warningf(logger.EventNotificationError, "Failed to send cancellation notification: %v", err)
				} else {
					s.logger.Infof(logger.EventHibernationWarningCancel, "Cancellation notification sent: activity detected")
				}
			}
		}
	}
}

func (s *AutoHibernateService) checkAndHibernate() (shouldWarn bool, isHibernating bool) {
	s.logger.Debug(logger.EventIdleCheckInfo, "Starting idle state check")

	result, err := s.idleMonitor.Check(s.logger)
	if err != nil {
		s.logger.Errorf(logger.EventIdleCheckError, "Error checking idle state: %v", err)
		return false, false
	}

	s.logger.Debugf(logger.EventIdleCheckInfo, "Idle check result: ShouldWarn=%v, ShouldHibernate=%v, Reason=%s",
		result.ShouldWarn, result.ShouldHibernate, result.Reason)

	if result.ShouldWarn {
		// In warning period - send notification (throttled)
		now := time.Now()
		timeSinceLastNotification := now.Sub(s.lastNotificationTime)

		if timeSinceLastNotification >= notificationThrottleDuration || s.lastNotificationTime.IsZero() {
			s.logger.Debugf(logger.EventHibernationWarningSent, "Sending hibernation warning: %s (time remaining: %v)",
				result.Reason, result.TimeRemaining.Round(time.Second))

			if s.notifierManager != nil {
				err := s.notifierManager.SendWarning(result.Reason, result.TimeRemaining)
				if err != nil {
					s.logger.Warningf(logger.EventNotificationError, "Failed to send warning notification: %v", err)
				} else {
					s.lastNotificationTime = now
					s.logger.Infof(logger.EventHibernationWarningSent, "Warning sent: %s (time remaining: %v)",
						result.Reason, result.TimeRemaining.Round(time.Second))
				}
			}
		} else {
			s.logger.Debugf(logger.EventHibernationWarningSent, "Skipping notification (throttled): %s (last sent %v ago)",
				result.Reason, timeSinceLastNotification.Round(time.Second))
		}
		return true, false
	} else if result.ShouldHibernate {
		// Warning period expired or no warning configured - hibernate now
		s.logger.Infof(logger.EventHibernationTriggered, "Hibernation triggered: %s", result.Reason)
		s.logger.Debug(logger.EventHibernationTriggered, "Initiating Azure hibernation API call")

		// Reset idle monitor state before hibernation
		// This ensures clean state when VM resumes from hibernation
		s.idleMonitor.Reset()
		s.logger.Debug(logger.EventHibernationTriggered, "Idle monitor state reset for clean resume")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := s.azureClient.HibernateVM(ctx); err != nil {
			s.logger.Errorf(logger.EventHibernationError, "Failed to hibernate VM: %v", err)
			return false, false
		}

		s.logger.Info(logger.EventHibernationSuccess, "Hibernation request sent successfully")
		// The VM will hibernate, service will stop
		return false, true
	} else {
		s.logger.Debug(logger.EventIdleCheckInfo, "System is active, no hibernation needed")
		return false, false
	}
}

// updateLoop periodically checks for updates when auto-update is enabled
func (s *AutoHibernateService) updateLoop() {
	checkInterval := time.Duration(s.config.UpdateCheckIntervalHr) * time.Hour
	s.logger.Infof(logger.EventServiceStart, "Auto-update enabled, checking for updates every %v", checkInterval)

	// Initial check after a short delay to allow service to fully start
	initialDelay := 1 * time.Minute
	select {
	case <-time.After(initialDelay):
	case <-s.stopChan:
		return
	}

	// Perform initial check
	s.checkAndApplyUpdate()

	// Then check periodically
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.checkAndApplyUpdate()
		case <-s.stopChan:
			s.logger.Info(logger.EventServiceStop, "Update loop stopping")
			return
		}
	}
}

// checkAndApplyUpdate checks for updates and applies them if available
func (s *AutoHibernateService) checkAndApplyUpdate() {
	s.logger.Debug(logger.EventServiceStart, "Checking for updates...")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	info, err := updater.CheckForUpdate(ctx)
	if err != nil {
		s.logger.Warningf(logger.EventConfigError, "Failed to check for updates: %v", err)
		return
	}

	if !info.UpdateAvailable {
		s.logger.Debug(logger.EventServiceStart, "No updates available")
		return
	}

	s.logger.Infof(logger.EventServiceStart, "Update available: %s -> %s", info.CurrentVersion, info.LatestVersion)

	// Download the update
	s.logger.Info(logger.EventServiceStart, "Downloading update...")
	tempDir, err := updater.DownloadUpdate(ctx)
	if err != nil {
		s.logger.Errorf(logger.EventConfigError, "Failed to download update: %v", err)
		return
	}

	s.logger.Infof(logger.EventServiceStart, "Update downloaded to %s", tempDir)

	// Trigger the update (spawns helper which will stop the service)
	s.logger.Info(logger.EventServiceStart, "Triggering update process...")
	if err := updater.TriggerUpdate(tempDir); err != nil {
		s.logger.Errorf(logger.EventConfigError, "Failed to trigger update: %v", err)
		return
	}

	// Mark that an update is pending - the updater will stop this service externally
	s.updatePending = true
	s.logger.Info(logger.EventServiceStop, "Update triggered, updater will stop and restart the service")
}

// Run executes the service
func Run(cfg *config.Config, vmMetadata *azure.VMMetadata, log logger.Logger, isDebug bool) error {
	service := NewAutoHibernateService(cfg, vmMetadata, log)

	if isDebug {
		// Run in debug mode (console)
		return debug.Run(appinfo.ServiceName, service)
	}

	// Run as Windows service
	return svc.Run(appinfo.ServiceName, service)
}
