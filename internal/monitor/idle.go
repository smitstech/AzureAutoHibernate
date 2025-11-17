//go:build windows

package monitor

import (
	"fmt"
	"time"

	"github.com/smitstech/AzureAutoHibernate/internal/logger"
)

const (
	// recentActivityThreshold is the duration used to detect recent user activity
	// Activity within this threshold cancels active hibernation warnings
	recentActivityThreshold = 30 * time.Second
)

// IdleCondition represents the type of idle condition that triggered
type IdleCondition int

const (
	IdleConditionNone            IdleCondition = iota // No idle condition met
	IdleConditionNoUsers                              // No users logged in
	IdleConditionAllDisconnected                      // All users disconnected
	IdleConditionInactiveUser                         // User logged in but inactive
)

// WarningState represents the current warning FSM state
type WarningState int

const (
	WarningStateNone     WarningState = iota // No warning active
	WarningStateActive                       // Warning issued, waiting for expiry or cancellation
	WarningStateCanceled                     // Warning was canceled due to user activity
)

type IdleState struct {
	NoUsersIdleSince     *time.Time
	AllDisconnectedSince *time.Time
	LastActivityTime     time.Time
	CurrentSessions      []SessionInfo
	IdleCondition        IdleCondition // Type of idle condition that triggered
	WarningIssuedAt      *time.Time
	WarningReason        string
	WarningState         WarningState
}

type IdleMonitor struct {
	state                    IdleState
	noUsersThreshold         time.Duration
	allDisconnectedThreshold time.Duration
	inactiveUserThreshold    time.Duration
	warningPeriod            time.Duration
	minimumUptimeThreshold   time.Duration
	resumeAt                 time.Time // Tracks when system resumed from hibernate/sleep
}

func NewIdleMonitor(noUsersMinutes, allDisconnectedMinutes, inactiveUserMinutes, inactiveUserWarningMinutes, minimumUptimeMinutes int) *IdleMonitor {
	now := time.Now()
	return &IdleMonitor{
		state: IdleState{
			LastActivityTime: now,
		},
		noUsersThreshold:         time.Duration(noUsersMinutes) * time.Minute,
		allDisconnectedThreshold: time.Duration(allDisconnectedMinutes) * time.Minute,
		inactiveUserThreshold:    time.Duration(inactiveUserMinutes) * time.Minute,
		warningPeriod:            time.Duration(inactiveUserWarningMinutes) * time.Minute,
		minimumUptimeThreshold:   time.Duration(minimumUptimeMinutes) * time.Minute,
		resumeAt:                 now, // Initialize to creation time
	}
}

// SetResumeTime updates the resume timestamp (called on power resume events)
func (m *IdleMonitor) SetResumeTime(t time.Time) {
	m.resumeAt = t
}

// Logger interface for idle monitor logging
type Logger interface {
	Debugf(eventID uint32, format string, args ...interface{})
	Infof(eventID uint32, format string, args ...interface{})
}

// CheckResult represents the result of an idle check
type CheckResult struct {
	Condition       IdleCondition // Type of idle condition that triggered
	ShouldWarn      bool
	ShouldHibernate bool
	Reason          string
	TimeRemaining   time.Duration
}

// shouldCancelWarning checks if current system state indicates the warning should be canceled
// Returns true if user activity is detected that should cancel an active warning
func (m *IdleMonitor) shouldCancelWarning(sessions []SessionInfo, hasUsers, allDisconnected bool, log Logger) bool {
	// If no warning is active, nothing to cancel
	if m.state.WarningState != WarningStateActive {
		return false
	}

	// Check for condition changes that should cancel warning based on explicit condition type
	switch m.state.IdleCondition {
	case IdleConditionNoUsers:
		// Warning was for "no users" but users have now logged in
		// (This shouldn't normally happen since no-users hibernates immediately, but handle it anyway)
		if hasUsers {
			log.Debugf(logger.EventHibernationWarningCancel, "Users logged in after warning was issued")
			return true
		}

	case IdleConditionAllDisconnected:
		// Warning was for "all disconnected" but user has reconnected
		if !allDisconnected {
			log.Debugf(logger.EventHibernationWarningCancel, "User reconnected after warning was issued")
			return true
		}

	case IdleConditionInactiveUser:
		// Warning was for inactive user, check for recent activity
		// This is the main case - user has moved mouse or pressed key recently
		if hasUsers && !allDisconnected {
			// Check all active sessions for recent input
			for _, session := range sessions {
				if session.IsDisconnected {
					continue
				}

				sessionIdleTime, err := GetSessionIdleTime(session.SessionId)
				if err != nil {
					log.Debugf(logger.EventIdleCheckError, "Failed to get idle time for session %d: %v", session.SessionId, err)
					continue
				}

				// If any active session has recent input, cancel the warning
				if sessionIdleTime < recentActivityThreshold {
					log.Debugf(logger.EventUserActivity, "Recent activity detected in session %d (idle: %v), canceling warning",
						session.SessionId, sessionIdleTime.Round(time.Second))
					return true
				}
			}
		}
	}

	return false
}

// Check evaluates all idle conditions and returns the check result
func (m *IdleMonitor) Check(log Logger) (*CheckResult, error) {
	now := time.Now()

	// Get current sessions
	sessions, err := GetActiveSessions()
	if err != nil {
		return nil, fmt.Errorf("failed to get active sessions: %w", err)
	}
	m.state.CurrentSessions = sessions

	log.Debugf(logger.EventIdleCheckInfo, "Session check: %d session(s) found", len(sessions))
	for i, session := range sessions {
		log.Debugf(logger.EventIdleCheckInfo, "  Session %d: User=%s, SessionID=%d, State=%d, Disconnected=%v",
			i+1, session.Username, session.SessionId, session.State, session.IsDisconnected)
	}

	// Check minimum uptime threshold to prevent flapping after hibernation/reboot
	if m.minimumUptimeThreshold > 0 {
		// Get system uptime (time since boot)
		systemUptime, err := GetSystemUptime()
		if err != nil {
			log.Debugf(logger.EventIdleCheckError, "Failed to get system uptime: %v", err)
		} else {
			// Calculate time since resume from hibernation/sleep
			timeSinceResume := now.Sub(m.resumeAt)

			// Use the MINIMUM of system uptime and time since resume
			// This ensures we respect minimum uptime after BOTH reboots AND hibernate/resume cycles
			effectiveUptime := systemUptime
			if timeSinceResume < systemUptime {
				effectiveUptime = timeSinceResume
			}

			if effectiveUptime <= m.minimumUptimeThreshold {
				timeRemaining := m.minimumUptimeThreshold - effectiveUptime
				log.Debugf(logger.EventIdleCheckInfo, "Effective uptime %v has not exceeded minimum threshold %v (remaining: %v), skipping idle checks (system: %v, since resume: %v)",
					effectiveUptime.Round(time.Second), m.minimumUptimeThreshold, timeRemaining.Round(time.Second),
					systemUptime.Round(time.Second), timeSinceResume.Round(time.Second))
				return &CheckResult{
					Condition:       IdleConditionNone,
					ShouldWarn:      false,
					ShouldHibernate: false,
					TimeRemaining:   timeRemaining,
				}, nil
			}
			log.Debugf(logger.EventIdleCheckInfo, "Effective uptime: %v (system: %v, since resume: %v, minimum threshold: %v)",
				effectiveUptime.Round(time.Second), systemUptime.Round(time.Second), timeSinceResume.Round(time.Second), m.minimumUptimeThreshold)
		}
	}

	// Check if user became active during warning period
	hasUsers := len(sessions) > 0
	allDisconnected := true
	for _, session := range sessions {
		if !session.IsDisconnected {
			allDisconnected = false
			break
		}
	}

	log.Debugf(logger.EventSessionSummary, "Session summary: hasUsers=%v, allDisconnected=%v", hasUsers, allDisconnected)

	// FSM State Transition: Check if warning should be canceled due to user activity
	if m.shouldCancelWarning(sessions, hasUsers, allDisconnected, log) {
		log.Infof(logger.EventHibernationWarningCancel, "User activity detected, canceling hibernation warning")
		m.state.WarningState = WarningStateCanceled
		m.resetWarning()
	}

	var idleReason string
	var idleCondition IdleCondition = IdleConditionNone

	// Condition 1: No users logged in for threshold duration
	if !hasUsers {
		if m.state.NoUsersIdleSince == nil {
			m.state.NoUsersIdleSince = &now
			log.Infof(logger.EventIdleCheckInfo, "No users logged in, starting idle timer")
		} else {
			idleDuration := now.Sub(*m.state.NoUsersIdleSince)
			if idleDuration >= m.noUsersThreshold {
				idleCondition = IdleConditionNoUsers
				idleReason = fmt.Sprintf("No users logged in for over %d minutes", int(m.noUsersThreshold.Minutes()))
				log.Debugf(logger.EventIdleThresholdMet, "Idle threshold met: %s", idleReason)
			} else {
				log.Infof(logger.EventIdleCheckInfo, "No users logged in for %v (threshold: %v)", idleDuration.Round(time.Second), m.noUsersThreshold)
			}
		}
	} else {
		m.state.NoUsersIdleSince = nil
	}

	// Condition 2: All users disconnected for threshold duration
	if idleCondition == IdleConditionNone && allDisconnected && hasUsers {
		if m.state.AllDisconnectedSince == nil {
			m.state.AllDisconnectedSince = &now
			log.Infof(logger.EventIdleCheckInfo, "All users disconnected, starting idle timer")
		} else {
			idleDuration := now.Sub(*m.state.AllDisconnectedSince)
			if idleDuration >= m.allDisconnectedThreshold {
				idleCondition = IdleConditionAllDisconnected
				idleReason = fmt.Sprintf("All users disconnected for over %d minutes", int(m.allDisconnectedThreshold.Minutes()))
				log.Debugf(logger.EventIdleThresholdMet, "Idle threshold met: %s", idleReason)
			} else {
				log.Infof(logger.EventIdleCheckInfo, "All users disconnected for %v (threshold: %v)", idleDuration.Round(time.Second), m.allDisconnectedThreshold)
			}
		}
	} else if !allDisconnected {
		if m.state.AllDisconnectedSince != nil {
			log.Debugf(logger.EventUserActivity, "User reconnected, resetting AllDisconnectedSince timer")
		}
		m.state.AllDisconnectedSince = nil
	} else if idleCondition != IdleConditionNone {
		// Condition 2 skipped because another condition was already met
		log.Debugf(logger.EventIdleCheckInfo, "Skipping 'all disconnected' check: condition already set to %d", idleCondition)
	}

	// Condition 3: User logged in but no input activity for threshold duration
	// Only check if there are active (non-disconnected) sessions
	hasActiveSessions := false
	if hasUsers && !allDisconnected {
		for _, session := range sessions {
			if !session.IsDisconnected {
				hasActiveSessions = true
				break
			}
		}
	}

	if idleCondition == IdleConditionNone && hasActiveSessions {
		// Check idle time for each active (non-disconnected) session
		// We use the MINIMUM idle time across all sessions (most recent activity)
		minIdleDuration := time.Duration(0)
		activeSessionCount := 0

		for _, session := range sessions {
			if session.IsDisconnected {
				continue
			}

			sessionIdleTime, err := GetSessionIdleTime(session.SessionId)
			if err != nil {
				log.Debugf(logger.EventIdleCheckError, "Failed to get idle time for session %d (%s): %v", session.SessionId, session.Username, err)
				continue
			}

			log.Debugf(logger.EventIdleCheckInfo, "Session %d (%s): idle for %v", session.SessionId, session.Username, sessionIdleTime.Round(time.Second))

			if activeSessionCount == 0 || sessionIdleTime < minIdleDuration {
				minIdleDuration = sessionIdleTime
			}
			activeSessionCount++
		}

		if activeSessionCount == 0 {
			log.Debugf(logger.EventIdleCheckInfo, "No active sessions to check for input activity")
		} else {
			lastInputTime := now.Add(-minIdleDuration)
			m.state.LastActivityTime = lastInputTime

			log.Debugf(logger.EventUserActivity, "User input activity: LastInput=%s, IdleFor=%v, Threshold=%v",
				lastInputTime.Format("15:04:05"), minIdleDuration.Round(time.Second), m.inactiveUserThreshold)

			if minIdleDuration >= m.inactiveUserThreshold {
				idleCondition = IdleConditionInactiveUser
				idleReason = fmt.Sprintf("No activity detected for over %d minutes", int(m.inactiveUserThreshold.Minutes()))
				log.Debugf(logger.EventIdleThresholdMet, "Idle condition met: %s", idleReason)
			} else {
				log.Infof(logger.EventIdleCheckInfo, "User idle for %v (threshold: %v)", minIdleDuration.Round(time.Second), m.inactiveUserThreshold)
			}
		}
	}

	// No idle condition met
	if idleCondition == IdleConditionNone {
		// FSM State Transition: Active -> None (condition no longer met)
		// If we were in a warning period but no condition is met anymore, reset warning
		if m.state.WarningIssuedAt != nil {
			log.Infof(logger.EventIdleConditionNoLongerMet, "FSM: Idle condition no longer met, resetting warning state")
			m.resetWarning()
		}
		return &CheckResult{
			Condition:       IdleConditionNone,
			ShouldWarn:      false,
			ShouldHibernate: false,
		}, nil
	}

	// Store the current idle condition in state
	m.state.IdleCondition = idleCondition

	// Idle condition met - determine how to handle based on condition type
	log.Debugf(logger.EventIdleThresholdMet, "Idle condition triggered: %s (type: %d)", idleReason, idleCondition)

	// Inactive user conditions get a warning period, others hibernate immediately
	if idleCondition == IdleConditionInactiveUser && m.warningPeriod > 0 {
		// Inactive user with warning period configured - warn before hibernating
		if m.state.WarningIssuedAt == nil {
			// FSM State Transition: None -> Active
			// Start warning period
			log.Debugf(logger.EventHibernationWarningStart, "FSM: Transition None -> Active, starting warning period (%v)", m.warningPeriod)
			m.state.WarningIssuedAt = &now
			m.state.WarningReason = idleReason
			m.state.WarningState = WarningStateActive
			return &CheckResult{
				Condition:       idleCondition,
				ShouldWarn:      true,
				ShouldHibernate: false,
				Reason:          idleReason,
				TimeRemaining:   m.warningPeriod,
			}, nil
		} else {
			// Warning already issued - check if warning period expired
			warnDuration := now.Sub(*m.state.WarningIssuedAt)
			log.Debugf(logger.EventWarningPeriodActive, "Warning period elapsed: %v / %v", warnDuration.Round(time.Second), m.warningPeriod)
			if warnDuration >= m.warningPeriod {
				// FSM State Transition: Active -> Hibernate
				// Warning period expired, hibernate now
				log.Debugf(logger.EventHibernationTriggered, "FSM: Warning period expired, proceeding with hibernation")
				return &CheckResult{
					Condition:       idleCondition,
					ShouldWarn:      false,
					ShouldHibernate: true,
					Reason:          idleReason,
					TimeRemaining:   0,
				}, nil
			} else {
				// Still in warning period, maintain Active state
				timeRemaining := m.warningPeriod - warnDuration
				log.Debugf(logger.EventWarningPeriodActive, "FSM: Still in Active state, %v remaining", timeRemaining.Round(time.Second))
				return &CheckResult{
					Condition:       idleCondition,
					ShouldWarn:      true,
					ShouldHibernate: false,
					Reason:          idleReason,
					TimeRemaining:   timeRemaining,
				}, nil
			}
		}
	} else {
		// No users or all disconnected - hibernate immediately (no one to warn)
		log.Debugf(logger.EventHibernationTriggered, "No active users to warn, hibernating immediately (condition: %d)", idleCondition)
		return &CheckResult{
			Condition:       idleCondition,
			ShouldWarn:      false,
			ShouldHibernate: true,
			Reason:          idleReason,
			TimeRemaining:   0,
		}, nil
	}
}

// resetWarning resets the warning state to None
func (m *IdleMonitor) resetWarning() {
	m.state.IdleCondition = IdleConditionNone
	m.state.WarningIssuedAt = nil
	m.state.WarningReason = ""
	m.state.WarningState = WarningStateNone
	m.state.NoUsersIdleSince = nil
	m.state.AllDisconnectedSince = nil
}

// Reset completely resets all idle monitor state
// This should be called before hibernation to ensure clean state after resume
func (m *IdleMonitor) Reset() {
	m.state.IdleCondition = IdleConditionNone
	m.state.WarningIssuedAt = nil
	m.state.WarningReason = ""
	m.state.WarningState = WarningStateNone
	m.state.NoUsersIdleSince = nil
	m.state.AllDisconnectedSince = nil
	m.state.LastActivityTime = time.Now()
	m.state.CurrentSessions = nil
}

// GetState returns the current idle state for debugging/monitoring
func (m *IdleMonitor) GetState() IdleState {
	return m.state
}

// GetTimeUntilThresholds returns the time remaining until each enabled threshold
// Returns the minimum time until any threshold is reached, or 0 if already exceeded
func (m *IdleMonitor) GetTimeUntilThresholds() (time.Duration, error) {
	now := time.Now()
	minTimeUntil := time.Duration(0)
	hasActiveCondition := false

	// Check condition 1: No users logged in
	if m.noUsersThreshold > 0 && m.state.NoUsersIdleSince != nil {
		elapsed := now.Sub(*m.state.NoUsersIdleSince)
		timeUntil := m.noUsersThreshold - elapsed
		// Clamp to 0 if threshold already exceeded (negative time)
		if timeUntil < 0 {
			timeUntil = 0
		}
		if !hasActiveCondition || timeUntil < minTimeUntil {
			minTimeUntil = timeUntil
			hasActiveCondition = true
		}
	}

	// Check condition 2: All users disconnected
	if m.allDisconnectedThreshold > 0 && m.state.AllDisconnectedSince != nil {
		elapsed := now.Sub(*m.state.AllDisconnectedSince)
		timeUntil := m.allDisconnectedThreshold - elapsed
		// Clamp to 0 if threshold already exceeded (negative time)
		if timeUntil < 0 {
			timeUntil = 0
		}
		if !hasActiveCondition || timeUntil < minTimeUntil {
			minTimeUntil = timeUntil
			hasActiveCondition = true
		}
	}

	// Check condition 3: User inactive (need to get current session idle times)
	if m.inactiveUserThreshold > 0 && len(m.state.CurrentSessions) > 0 {
		// Check if there are any non-disconnected sessions
		hasActiveSession := false
		for _, session := range m.state.CurrentSessions {
			if !session.IsDisconnected {
				hasActiveSession = true
				break
			}
		}

		if hasActiveSession {
			// Get the minimum idle time across all active sessions
			minSessionIdle := time.Duration(0)
			foundSession := false

			for _, session := range m.state.CurrentSessions {
				if session.IsDisconnected {
					continue
				}

				sessionIdleTime, err := GetSessionIdleTime(session.SessionId)
				if err != nil {
					continue
				}

				if !foundSession || sessionIdleTime < minSessionIdle {
					minSessionIdle = sessionIdleTime
					foundSession = true
				}
			}

			if foundSession {
				timeUntil := m.inactiveUserThreshold - minSessionIdle
				// Clamp to 0 if threshold already exceeded (negative time)
				if timeUntil < 0 {
					timeUntil = 0
				}
				if !hasActiveCondition || timeUntil < minTimeUntil {
					minTimeUntil = timeUntil
					hasActiveCondition = true
				}
			}
		}
	}

	if !hasActiveCondition {
		// No conditions are currently active, return a default
		return 0, nil
	}

	// Clamp final result to 0 (should already be non-negative from above)
	if minTimeUntil < 0 {
		minTimeUntil = 0
	}

	return minTimeUntil, nil
}
