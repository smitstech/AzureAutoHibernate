//go:build windows

package service

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unsafe"

	"github.com/smitstech/AzureAutoHibernate/internal/appinfo"
	"github.com/smitstech/AzureAutoHibernate/internal/logger"
	"github.com/smitstech/AzureAutoHibernate/internal/monitor"
	"github.com/smitstech/AzureAutoHibernate/internal/pipe"
	"golang.org/x/sys/windows"
)

const (
	SE_TCB_NAME = "SeTcbPrivilege"
)

var (
	advapi32                    = windows.NewLazySystemDLL("advapi32.dll")
	procCreateProcessAsUser     = advapi32.NewProc("CreateProcessAsUserW")
	procLookupPrivilegeValue    = advapi32.NewProc("LookupPrivilegeValueW")
	procAdjustTokenPrivileges   = advapi32.NewProc("AdjustTokenPrivileges")
	wtsapi32                    = windows.NewLazySystemDLL("wtsapi32.dll")
	procWTSQueryUserToken       = wtsapi32.NewProc("WTSQueryUserToken")
	userenv                     = windows.NewLazySystemDLL("userenv.dll")
	procCreateEnvironmentBlock  = userenv.NewProc("CreateEnvironmentBlock")
	procDestroyEnvironmentBlock = userenv.NewProc("DestroyEnvironmentBlock")
)

type LUID struct {
	LowPart  uint32
	HighPart int32
}

type LUID_AND_ATTRIBUTES struct {
	Luid       LUID
	Attributes uint32
}

type TOKEN_PRIVILEGES struct {
	PrivilegeCount uint32
	Privileges     [1]LUID_AND_ATTRIBUTES
}

// NotifierProcess represents a running notifier process
type NotifierProcess struct {
	SessionID   int
	ProcessID   uint32
	Handle      windows.Handle
	PipeServer  *pipe.Server
	IsConnected bool // true if session is active/connected, false if disconnected
}

// NotifierManager manages notifier processes for user sessions
type NotifierManager struct {
	notifiers               map[int]*NotifierProcess
	mu                      sync.RWMutex
	logger                  logger.Logger
	notifierExePath         string
	stopChan                chan struct{}
	wg                      sync.WaitGroup
	startupNotificationSent bool
}

// NewNotifierManager creates a new notifier manager
func NewNotifierManager(log logger.Logger) (*NotifierManager, error) {
	// Get the path to the notifier executable
	// It should be in the same directory as the service
	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable path: %w", err)
	}

	exeDir := filepath.Dir(exePath)
	notifierPath := filepath.Join(exeDir, appinfo.NotifierExeName)

	// Check if notifier exists
	if _, err := os.Stat(notifierPath); err != nil {
		return nil, fmt.Errorf("notifier executable not found at %s: %w", notifierPath, err)
	}

	return &NotifierManager{
		notifiers:       make(map[int]*NotifierProcess),
		logger:          log,
		notifierExePath: notifierPath,
		stopChan:        make(chan struct{}),
	}, nil
}

// Start begins managing notifier processes
func (nm *NotifierManager) Start() error {
	nm.logger.Info(logger.EventServiceStart, "Starting notifier manager")

	// Enable SE_TCB_NAME privilege required for WTSQueryUserToken
	if err := nm.enableTcbPrivilege(); err != nil {
		return fmt.Errorf("failed to enable SE_TCB_NAME privilege: %w", err)
	}

	// Start session monitoring loop
	nm.wg.Add(1)
	go nm.monitorSessions()

	return nil
}

// Stop stops the notifier manager and all notifier processes
func (nm *NotifierManager) Stop() {
	nm.logger.Info(logger.EventServiceStop, "Stopping notifier manager")
	close(nm.stopChan)
	nm.wg.Wait()

	// Stop all notifiers
	nm.mu.Lock()
	defer nm.mu.Unlock()

	for sessionID, notifier := range nm.notifiers {
		nm.stopNotifier(sessionID, notifier)
	}
}

// enableTcbPrivilege enables the SE_TCB_NAME privilege required for WTSQueryUserToken
func (nm *NotifierManager) enableTcbPrivilege() error {
	const SE_PRIVILEGE_ENABLED = uint32(0x00000002)

	// Get current process token
	var token windows.Token
	err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &token)
	if err != nil {
		return fmt.Errorf("OpenProcessToken failed: %w", err)
	}
	defer token.Close()

	// Check if running as SYSTEM by checking the user SID
	user, err := token.GetTokenUser()
	if err == nil {
		// Well-known SID for SYSTEM: S-1-5-18
		systemSid, _ := windows.StringToSid("S-1-5-18")
		if !user.User.Sid.Equals(systemSid) {
			nm.logger.Warningf(logger.EventSessionInfoWarning,
				"Not running as SYSTEM - notifier processes cannot be launched in debug/console mode")
			nm.logger.Warningf(logger.EventSessionInfoWarning,
				"User notifications will not work. Run as a Windows service for full functionality.")
			return fmt.Errorf("SE_TCB_NAME privilege requires running as SYSTEM (not available in debug mode)")
		}
	}

	// Lookup the LUID for SE_TCB_NAME privilege
	var luid LUID
	privilegeName, err := windows.UTF16PtrFromString(SE_TCB_NAME)
	if err != nil {
		return fmt.Errorf("failed to convert privilege name: %w", err)
	}

	ret, _, err := procLookupPrivilegeValue.Call(
		0, // local system
		uintptr(unsafe.Pointer(privilegeName)),
		uintptr(unsafe.Pointer(&luid)),
	)
	if ret == 0 {
		return fmt.Errorf("LookupPrivilegeValue failed: %w", err)
	}

	// Enable the privilege
	tp := TOKEN_PRIVILEGES{
		PrivilegeCount: 1,
		Privileges: [1]LUID_AND_ATTRIBUTES{
			{
				Luid:       luid,
				Attributes: SE_PRIVILEGE_ENABLED,
			},
		},
	}

	ret, _, callErr := procAdjustTokenPrivileges.Call(
		uintptr(token),
		0, // do not disable all privileges
		uintptr(unsafe.Pointer(&tp)),
		0,
		0,
		0,
	)
	if ret == 0 {
		return fmt.Errorf("AdjustTokenPrivileges failed: %w", callErr)
	}

	// Even if ret != 0, check if not all privileges were assigned
	// AdjustTokenPrivileges can succeed but not grant the privilege
	lastErr := windows.GetLastError()
	if lastErr == windows.ERROR_NOT_ALL_ASSIGNED {
		return fmt.Errorf("SE_TCB_NAME privilege was not assigned (need to run as SYSTEM)")
	}

	nm.logger.Info(logger.EventServiceStart, "SE_TCB_NAME privilege enabled successfully")
	return nil
}

// monitorSessions performs initial session check and setup
// Notifier health is now checked on-demand before sending notifications
func (nm *NotifierManager) monitorSessions() {
	defer nm.wg.Done()

	// Do an initial check to start notifiers for existing sessions
	nm.checkSessions()
}

// checkSessions checks for active sessions and ensures notifiers are running
func (nm *NotifierManager) checkSessions() {
	sessions, err := monitor.GetActiveSessions()
	if err != nil {
		nm.logger.Errorf(logger.EventSessionMonitorError, "Failed to get active sessions: %v", err)
		return
	}

	nm.mu.Lock()

	// Track which sessions are active
	activeSessions := make(map[int]bool)
	connectedNotifierStarted := false

	for _, session := range sessions {
		sessionID := int(session.SessionId)
		activeSessions[sessionID] = true
		isConnected := session.IsActive // session is connected (not disconnected)

		nm.logger.Debugf(logger.EventSessionMonitorError, "Session %d: State=%d, IsActive=%v, IsConnected=%v",
			sessionID, session.State, session.IsActive, isConnected)

		// Check if notifier is already running for this session
		if notifier, exists := nm.notifiers[sessionID]; exists {
			// Update connection state
			notifier.IsConnected = isConnected

			// Check if process is still alive
			if nm.isProcessAlive(notifier.Handle) {
				continue
			}
			// Process died, clean up
			nm.logger.Infof(logger.EventMonitoringStarted, "Notifier process for session %d died, restarting", sessionID)
			nm.stopNotifier(sessionID, notifier)
		}

		// Start notifier for this session
		err := nm.startNotifier(sessionID, isConnected)
		if err != nil {
			nm.logger.Errorf(logger.EventSessionMonitorError, "Failed to start notifier for session %d: %v", sessionID, err)
		} else {
			// Track if we started a connected notifier (for startup notification)
			if isConnected {
				connectedNotifierStarted = true
			}
		}
	}

	// Stop notifiers for sessions that are no longer active
	for sessionID, notifier := range nm.notifiers {
		if !activeSessions[sessionID] {
			nm.logger.Infof(logger.EventMonitoringStarted, "Session %d is no longer active, stopping notifier", sessionID)
			nm.stopNotifier(sessionID, notifier)
		}
	}

	// Check if we need to send startup notification (only on first call with connected session)
	var shouldSendStartupNotification bool
	if !nm.startupNotificationSent && connectedNotifierStarted && len(nm.notifiers) > 0 {
		shouldSendStartupNotification = true
		nm.startupNotificationSent = true
		nm.logger.Debugf(logger.EventServiceStart, "Startup notification will be sent to %d connected session(s)", len(nm.notifiers))
	}

	nm.mu.Unlock()

	// Send startup notification outside of lock to avoid deadlock
	if shouldSendStartupNotification {
		// Give the notifier a moment to fully initialize
		time.Sleep(500 * time.Millisecond)
		err := nm.SendInfo("Service started and monitoring for idle activity")
		if err != nil {
			nm.logger.Warningf(logger.EventSessionInfoWarning, "Failed to send startup notification: %v", err)
		}
	}
}

// startNotifier launches a notifier process for the given session
func (nm *NotifierManager) startNotifier(sessionID int, isConnected bool) error {
	nm.logger.Infof(logger.EventMonitoringStarted, "Starting notifier for session %d", sessionID)

	// Get user token for the session
	var userToken windows.Token
	ret, _, err := procWTSQueryUserToken.Call(
		uintptr(sessionID),
		uintptr(unsafe.Pointer(&userToken)),
	)
	if ret == 0 {
		return fmt.Errorf("WTSQueryUserToken failed: %w", err)
	}
	defer userToken.Close()

	// Duplicate the token to get a primary token
	var primaryToken windows.Token
	err = windows.DuplicateTokenEx(
		userToken,
		windows.MAXIMUM_ALLOWED,
		nil,
		windows.SecurityImpersonation,
		windows.TokenPrimary,
		&primaryToken,
	)
	if err != nil {
		return fmt.Errorf("DuplicateTokenEx failed: %w", err)
	}
	defer primaryToken.Close()

	// Create environment block for the user
	var envBlock uintptr
	ret, _, err = procCreateEnvironmentBlock.Call(
		uintptr(unsafe.Pointer(&envBlock)),
		uintptr(primaryToken),
		0, // Don't inherit current environment
	)
	if ret == 0 {
		return fmt.Errorf("CreateEnvironmentBlock failed: %w", err)
	}
	defer procDestroyEnvironmentBlock.Call(envBlock)

	// Prepare command line
	cmdLine, err := windows.UTF16PtrFromString(fmt.Sprintf(`"%s" -session %d`, nm.notifierExePath, sessionID))
	if err != nil {
		return fmt.Errorf("failed to create command line: %w", err)
	}

	// Prepare startup info
	var si windows.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Desktop, _ = windows.UTF16PtrFromString("winsta0\\default")

	// Create process as user
	var pi windows.ProcessInformation
	ret, _, err = procCreateProcessAsUser.Call(
		uintptr(primaryToken),
		0, // No application name
		uintptr(unsafe.Pointer(cmdLine)),
		0, // Process security attributes
		0, // Thread security attributes
		0, // Don't inherit handles
		windows.CREATE_NEW_CONSOLE|windows.CREATE_UNICODE_ENVIRONMENT,
		envBlock,
		0, // Current directory (use default)
		uintptr(unsafe.Pointer(&si)),
		uintptr(unsafe.Pointer(&pi)),
	)
	if ret == 0 {
		return fmt.Errorf("CreateProcessAsUser failed: %w", err)
	}

	// Close thread handle (we don't need it)
	windows.CloseHandle(pi.Thread)

	// Create pipe server for this session
	pipeServer := pipe.NewServer(sessionID, nm.logger)

	// Store notifier process info
	nm.notifiers[sessionID] = &NotifierProcess{
		SessionID:   sessionID,
		ProcessID:   pi.ProcessId,
		Handle:      pi.Process,
		PipeServer:  pipeServer,
		IsConnected: isConnected,
	}

	nm.logger.Infof(logger.EventMonitoringStarted, "Notifier started for session %d (PID: %d)", sessionID, pi.ProcessId)

	// Wait a moment for the notifier to start and create its pipe
	time.Sleep(1 * time.Second)

	// Ping the notifier to verify it's working
	err = pipeServer.Ping()
	if err != nil {
		nm.logger.Warningf(logger.EventSessionInfoWarning, "Failed to ping notifier for session %d: %v", sessionID, err)
	} else {
		nm.logger.Debugf(logger.EventMonitoringStarted, "Notifier for session %d is responding", sessionID)
	}

	return nil
}

// stopNotifier stops a notifier process
func (nm *NotifierManager) stopNotifier(sessionID int, notifier *NotifierProcess) {
	nm.logger.Infof(logger.EventMonitoringStarted, "Stopping notifier for session %d", sessionID)

	// Terminate the process
	windows.TerminateProcess(notifier.Handle, 0)
	windows.CloseHandle(notifier.Handle)

	delete(nm.notifiers, sessionID)
}

// isProcessAlive checks if a process is still running
func (nm *NotifierManager) isProcessAlive(handle windows.Handle) bool {
	var exitCode uint32
	err := windows.GetExitCodeProcess(handle, &exitCode)
	if err != nil {
		return false
	}
	// STILL_ACTIVE = 259
	return exitCode == 259
}

// ensureNotifiersReady checks sessions and ensures notifiers are running
// This is called on-demand before sending notifications
func (nm *NotifierManager) ensureNotifiersReady() {
	nm.checkSessions()
}

// SendWarning sends a warning notification to all connected sessions
func (nm *NotifierManager) SendWarning(reason string, timeRemaining time.Duration) error {
	// Ensure notifiers are running before sending
	nm.ensureNotifiersReady()

	nm.mu.RLock()
	defer nm.mu.RUnlock()

	if len(nm.notifiers) == 0 {
		nm.logger.Debug(logger.EventIdleCheckInfo, "No active notifiers to send warning to")
		return nil
	}

	cmd := pipe.NotifyCommand{
		Type:          pipe.CommandWarning,
		TimeRemaining: int(timeRemaining.Seconds()),
		Reason:        reason,
		Message:       pipe.FormatWarningMessage(reason, timeRemaining),
		Timestamp:     time.Now(),
	}

	var lastErr error
	successCount := 0

	for sessionID, notifier := range nm.notifiers {
		// Only send to connected sessions (warnings are only relevant to active users)
		if !notifier.IsConnected {
			nm.logger.Debugf(logger.EventHibernationWarningSent, "Skipping warning to disconnected session %d", sessionID)
			continue
		}

		_, err := notifier.PipeServer.SendCommand(cmd)
		if err != nil {
			nm.logger.Warningf(logger.EventSessionInfoWarning, "Failed to send warning to session %d: %v", sessionID, err)
			lastErr = err
		} else {
			successCount++
		}
	}

	if successCount > 0 {
		nm.logger.Infof(logger.EventHibernationWarningSent, "Warning sent to %d connected session(s)", successCount)
	}

	return lastErr
}

// SendCancellation sends a cancellation notification to all connected sessions
func (nm *NotifierManager) SendCancellation() error {
	// Ensure notifiers are running before sending
	nm.ensureNotifiersReady()

	nm.mu.RLock()
	defer nm.mu.RUnlock()

	if len(nm.notifiers) == 0 {
		return nil
	}

	cmd := pipe.NotifyCommand{
		Type:      pipe.CommandCancel,
		Message:   pipe.FormatCancellationMessage(),
		Timestamp: time.Now(),
	}

	var lastErr error
	successCount := 0

	for sessionID, notifier := range nm.notifiers {
		// Only send to connected sessions (cancellation is only relevant to active users)
		if !notifier.IsConnected {
			nm.logger.Debugf(logger.EventHibernationWarningCancel, "Skipping cancellation to disconnected session %d", sessionID)
			continue
		}

		_, err := notifier.PipeServer.SendCommand(cmd)
		if err != nil {
			nm.logger.Warningf(logger.EventSessionInfoWarning, "Failed to send cancellation to session %d: %v", sessionID, err)
			lastErr = err
		} else {
			successCount++
		}
	}

	if successCount > 0 {
		nm.logger.Infof(logger.EventMonitoringStarted, "Cancellation sent to %d connected session(s)", successCount)
	}

	return lastErr
}

// DismissWarning sends a dismiss command to all active sessions
func (nm *NotifierManager) DismissWarning() error {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	if len(nm.notifiers) == 0 {
		return nil
	}

	cmd := pipe.NotifyCommand{
		Type:      pipe.CommandDismiss,
		Timestamp: time.Now(),
	}

	for sessionID, notifier := range nm.notifiers {
		err := notifier.PipeServer.SendCommandNoWait(cmd)
		if err != nil {
			nm.logger.Debugf(logger.EventMonitoringStarted, "Failed to send dismiss to session %d: %v", sessionID, err)
		}
	}

	return nil
}

// SendInfo sends an informational notification to all connected sessions
func (nm *NotifierManager) SendInfo(message string) error {
	// Ensure notifiers are running before sending
	nm.ensureNotifiersReady()

	nm.mu.RLock()
	defer nm.mu.RUnlock()

	if len(nm.notifiers) == 0 {
		nm.logger.Debug(logger.EventServiceStart, "No active notifiers to send info to")
		return nil
	}

	cmd := pipe.NotifyCommand{
		Type:      pipe.CommandInfo,
		Message:   message,
		Timestamp: time.Now(),
	}

	var lastErr error
	successCount := 0
	skippedCount := 0

	for sessionID, notifier := range nm.notifiers {
		// Only send to connected sessions
		if !notifier.IsConnected {
			skippedCount++
			nm.logger.Debugf(logger.EventServiceStart, "Skipping info notification to disconnected session %d", sessionID)
			continue
		}

		_, err := notifier.PipeServer.SendCommand(cmd)
		if err != nil {
			nm.logger.Warningf(logger.EventSessionInfoWarning, "Failed to send info to session %d: %v", sessionID, err)
			lastErr = err
		} else {
			successCount++
		}
	}

	if successCount > 0 {
		nm.logger.Infof(logger.EventServiceStart, "Info notification sent to %d connected session(s): %s", successCount, message)
	}
	if skippedCount > 0 {
		nm.logger.Debugf(logger.EventServiceStart, "Skipped %d disconnected session(s)", skippedCount)
	}

	return lastErr
}
