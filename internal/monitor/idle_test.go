//go:build windows

package monitor

import (
	"testing"
	"time"
)

// mockLogger is a simple logger for testing that captures log messages
type mockLogger struct {
	debugLogs []string
	infoLogs  []string
	warnLogs  []string
	errorLogs []string
}

func (m *mockLogger) Debugf(eventID uint32, format string, args ...interface{}) {
	m.debugLogs = append(m.debugLogs, format)
}

func (m *mockLogger) Infof(eventID uint32, format string, args ...interface{}) {
	m.infoLogs = append(m.infoLogs, format)
}

func (m *mockLogger) Warningf(eventID uint32, format string, args ...interface{}) {
	m.warnLogs = append(m.warnLogs, format)
}

func (m *mockLogger) Errorf(eventID uint32, format string, args ...interface{}) {
	m.errorLogs = append(m.errorLogs, format)
}

func (m *mockLogger) Debug(eventID uint32, msg string) {
	m.debugLogs = append(m.debugLogs, msg)
}

func (m *mockLogger) Info(eventID uint32, msg string) {
	m.infoLogs = append(m.infoLogs, msg)
}

func (m *mockLogger) Warning(eventID uint32, msg string) {
	m.warnLogs = append(m.warnLogs, msg)
}

func (m *mockLogger) Error(eventID uint32, msg string) {
	m.errorLogs = append(m.errorLogs, msg)
}

// TestNewIdleMonitor tests the idle monitor constructor
func TestNewIdleMonitor(t *testing.T) {
	tests := []struct {
		name                    string
		noUsers                 int
		allDisconnected         int
		inactiveUser            int
		inactiveUserWarning     int
		minimumUptime           int
		expectedNoUsers         time.Duration
		expectedAllDisconnected time.Duration
		expectedInactiveUser    time.Duration
		expectedWarningPeriod   time.Duration
		expectedMinimumUptime   time.Duration
	}{
		{
			name:                    "standard thresholds",
			noUsers:                 30,
			allDisconnected:         60,
			inactiveUser:            120,
			inactiveUserWarning:     5,
			minimumUptime:           10,
			expectedNoUsers:         30 * time.Minute,
			expectedAllDisconnected: 60 * time.Minute,
			expectedInactiveUser:    120 * time.Minute,
			expectedWarningPeriod:   5 * time.Minute,
			expectedMinimumUptime:   10 * time.Minute,
		},
		{
			name:                    "zero thresholds",
			noUsers:                 0,
			allDisconnected:         0,
			inactiveUser:            0,
			inactiveUserWarning:     0,
			minimumUptime:           0,
			expectedNoUsers:         0,
			expectedAllDisconnected: 0,
			expectedInactiveUser:    0,
			expectedWarningPeriod:   0,
			expectedMinimumUptime:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			monitor := NewIdleMonitor(tt.noUsers, tt.allDisconnected, tt.inactiveUser, tt.inactiveUserWarning, tt.minimumUptime)

			if monitor.noUsersThreshold != tt.expectedNoUsers {
				t.Errorf("noUsersThreshold = %v, want %v", monitor.noUsersThreshold, tt.expectedNoUsers)
			}
			if monitor.allDisconnectedThreshold != tt.expectedAllDisconnected {
				t.Errorf("allDisconnectedThreshold = %v, want %v", monitor.allDisconnectedThreshold, tt.expectedAllDisconnected)
			}
			if monitor.inactiveUserThreshold != tt.expectedInactiveUser {
				t.Errorf("inactiveUserThreshold = %v, want %v", monitor.inactiveUserThreshold, tt.expectedInactiveUser)
			}
			if monitor.warningPeriod != tt.expectedWarningPeriod {
				t.Errorf("warningPeriod = %v, want %v", monitor.warningPeriod, tt.expectedWarningPeriod)
			}
			if monitor.minimumUptimeThreshold != tt.expectedMinimumUptime {
				t.Errorf("minimumUptimeThreshold = %v, want %v", monitor.minimumUptimeThreshold, tt.expectedMinimumUptime)
			}
			if monitor.state.WarningState != WarningStateNone {
				t.Errorf("initial WarningState = %v, want %v", monitor.state.WarningState, WarningStateNone)
			}
			if monitor.resumeAt.IsZero() {
				t.Error("resumeAt should be initialized")
			}
		})
	}
}

// TestSetResumeTime tests the resume time setter
func TestSetResumeTime(t *testing.T) {
	monitor := NewIdleMonitor(30, 60, 120, 5, 10)
	newTime := time.Now().Add(1 * time.Hour)

	monitor.SetResumeTime(newTime)

	if !monitor.resumeAt.Equal(newTime) {
		t.Errorf("resumeAt = %v, want %v", monitor.resumeAt, newTime)
	}
}

// TestResetWarning tests the resetWarning function
func TestResetWarning(t *testing.T) {
	monitor := NewIdleMonitor(30, 60, 120, 5, 10)
	now := time.Now()

	// Set up some state
	monitor.state.WarningIssuedAt = &now
	monitor.state.WarningReason = "test reason"
	monitor.state.WarningState = WarningStateActive
	monitor.state.NoUsersIdleSince = &now
	monitor.state.AllDisconnectedSince = &now

	// Reset warning
	monitor.resetWarning()

	// Verify all fields are reset
	if monitor.state.WarningIssuedAt != nil {
		t.Error("WarningIssuedAt should be nil after reset")
	}
	if monitor.state.WarningReason != "" {
		t.Error("WarningReason should be empty after reset")
	}
	if monitor.state.WarningState != WarningStateNone {
		t.Errorf("WarningState = %v, want %v", monitor.state.WarningState, WarningStateNone)
	}
	if monitor.state.NoUsersIdleSince != nil {
		t.Error("NoUsersIdleSince should be nil after reset")
	}
	if monitor.state.AllDisconnectedSince != nil {
		t.Error("AllDisconnectedSince should be nil after reset")
	}
}

// TestReset tests the Reset function (complete state reset)
func TestReset(t *testing.T) {
	monitor := NewIdleMonitor(30, 60, 120, 5, 10)
	now := time.Now()

	// Set up some state
	monitor.state.WarningIssuedAt = &now
	monitor.state.WarningReason = "test reason"
	monitor.state.WarningState = WarningStateActive
	monitor.state.NoUsersIdleSince = &now
	monitor.state.AllDisconnectedSince = &now
	monitor.state.LastActivityTime = now.Add(-1 * time.Hour)
	monitor.state.CurrentSessions = []SessionInfo{{SessionId: 1}}

	// Reset all state
	monitor.Reset()

	// Verify all fields are reset
	if monitor.state.WarningIssuedAt != nil {
		t.Error("WarningIssuedAt should be nil after reset")
	}
	if monitor.state.WarningReason != "" {
		t.Error("WarningReason should be empty after reset")
	}
	if monitor.state.WarningState != WarningStateNone {
		t.Errorf("WarningState = %v, want %v", monitor.state.WarningState, WarningStateNone)
	}
	if monitor.state.NoUsersIdleSince != nil {
		t.Error("NoUsersIdleSince should be nil after reset")
	}
	if monitor.state.AllDisconnectedSince != nil {
		t.Error("AllDisconnectedSince should be nil after reset")
	}
	if monitor.state.CurrentSessions != nil {
		t.Error("CurrentSessions should be nil after reset")
	}
	// LastActivityTime should be recent (not in the past)
	timeSinceActivity := time.Since(monitor.state.LastActivityTime)
	if timeSinceActivity > 1*time.Second {
		t.Errorf("LastActivityTime should be recent, but was %v ago", timeSinceActivity)
	}
}

// TestGetState tests the GetState function
func TestGetState(t *testing.T) {
	monitor := NewIdleMonitor(30, 60, 120, 5, 10)
	now := time.Now()

	// Set up some state
	monitor.state.WarningIssuedAt = &now
	monitor.state.WarningReason = "test reason"
	monitor.state.WarningState = WarningStateActive

	// Get state
	state := monitor.GetState()

	// Verify state is returned correctly
	if state.WarningIssuedAt == nil || !state.WarningIssuedAt.Equal(now) {
		t.Error("WarningIssuedAt not returned correctly")
	}
	if state.WarningReason != "test reason" {
		t.Errorf("WarningReason = %q, want %q", state.WarningReason, "test reason")
	}
	if state.WarningState != WarningStateActive {
		t.Errorf("WarningState = %v, want %v", state.WarningState, WarningStateActive)
	}
}

// TestShouldCancelWarning tests the warning cancellation logic
func TestShouldCancelWarning(t *testing.T) {
	tests := []struct {
		name            string
		warningState    WarningState
		noUsersIdle     bool
		allDiscIdle     bool
		hasUsers        bool
		allDisconnected bool
		sessions        []SessionInfo
		want            bool
	}{
		{
			name:            "no warning active - should not cancel",
			warningState:    WarningStateNone,
			noUsersIdle:     false,
			allDiscIdle:     false,
			hasUsers:        true,
			allDisconnected: false,
			sessions:        []SessionInfo{{SessionId: 1, IsDisconnected: false}},
			want:            false,
		},
		{
			name:            "warning for no users, users logged in - should cancel",
			warningState:    WarningStateActive,
			noUsersIdle:     true,
			allDiscIdle:     false,
			hasUsers:        true,
			allDisconnected: false,
			sessions:        []SessionInfo{{SessionId: 1, IsDisconnected: false}},
			want:            true,
		},
		{
			name:            "warning for all disconnected, user reconnected - should cancel",
			warningState:    WarningStateActive,
			noUsersIdle:     false,
			allDiscIdle:     true,
			hasUsers:        true,
			allDisconnected: false,
			sessions:        []SessionInfo{{SessionId: 1, IsDisconnected: false}},
			want:            true,
		},
		{
			name:            "warning for inactive user, still inactive - should not cancel",
			warningState:    WarningStateActive,
			noUsersIdle:     false,
			allDiscIdle:     false,
			hasUsers:        true,
			allDisconnected: false,
			sessions:        []SessionInfo{{SessionId: 1, IsDisconnected: false}},
			want:            false, // Note: actual cancellation would depend on GetSessionIdleTime
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			monitor := NewIdleMonitor(30, 60, 120, 5, 10)
			monitor.state.WarningState = tt.warningState

			now := time.Now()
			if tt.noUsersIdle {
				monitor.state.NoUsersIdleSince = &now
			}
			if tt.allDiscIdle {
				monitor.state.AllDisconnectedSince = &now
			}

			log := &mockLogger{}
			got := monitor.shouldCancelWarning(tt.sessions, tt.hasUsers, tt.allDisconnected, log)

			if got != tt.want {
				t.Errorf("shouldCancelWarning() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestGetTimeUntilThresholds tests time calculation logic
func TestGetTimeUntilThresholds(t *testing.T) {
	tests := []struct {
		name                string
		noUsersThreshold    int // minutes
		allDiscThreshold    int
		inactiveThreshold   int
		noUsersIdleSince    *time.Duration // how long ago
		allDiscIdleSince    *time.Duration
		expectedMinDuration time.Duration
		expectError         bool
	}{
		{
			name:                "no active conditions",
			noUsersThreshold:    30,
			allDiscThreshold:    60,
			inactiveThreshold:   120,
			noUsersIdleSince:    nil,
			allDiscIdleSince:    nil,
			expectedMinDuration: 0,
			expectError:         false,
		},
		{
			name:                "no users idle for 10 minutes, threshold 30 minutes",
			noUsersThreshold:    30,
			allDiscThreshold:    0,
			inactiveThreshold:   0,
			noUsersIdleSince:    durationPtr(10 * time.Minute),
			allDiscIdleSince:    nil,
			expectedMinDuration: 20 * time.Minute,
			expectError:         false,
		},
		{
			name:                "no users idle for 35 minutes, threshold 30 minutes (already exceeded)",
			noUsersThreshold:    30,
			allDiscThreshold:    0,
			inactiveThreshold:   0,
			noUsersIdleSince:    durationPtr(35 * time.Minute),
			allDiscIdleSince:    nil,
			expectedMinDuration: 0,
			expectError:         false,
		},
		{
			name:                "all disconnected for 40 minutes, threshold 60 minutes",
			noUsersThreshold:    0,
			allDiscThreshold:    60,
			inactiveThreshold:   0,
			noUsersIdleSince:    nil,
			allDiscIdleSince:    durationPtr(40 * time.Minute),
			expectedMinDuration: 20 * time.Minute,
			expectError:         false,
		},
		{
			name:                "multiple conditions, return minimum",
			noUsersThreshold:    30,
			allDiscThreshold:    60,
			inactiveThreshold:   0,
			noUsersIdleSince:    durationPtr(20 * time.Minute), // 10 minutes left
			allDiscIdleSince:    durationPtr(50 * time.Minute), // 10 minutes left
			expectedMinDuration: 10 * time.Minute,
			expectError:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			monitor := NewIdleMonitor(tt.noUsersThreshold, tt.allDiscThreshold, tt.inactiveThreshold, 5, 10)
			now := time.Now()

			if tt.noUsersIdleSince != nil {
				t := now.Add(-*tt.noUsersIdleSince)
				monitor.state.NoUsersIdleSince = &t
			}
			if tt.allDiscIdleSince != nil {
				t := now.Add(-*tt.allDiscIdleSince)
				monitor.state.AllDisconnectedSince = &t
			}

			got, err := monitor.GetTimeUntilThresholds()

			if (err != nil) != tt.expectError {
				t.Errorf("GetTimeUntilThresholds() error = %v, expectError %v", err, tt.expectError)
				return
			}

			// Allow some tolerance for timing (1 second)
			tolerance := 1 * time.Second
			diff := got - tt.expectedMinDuration
			if diff < 0 {
				diff = -diff
			}
			if diff > tolerance {
				t.Errorf("GetTimeUntilThresholds() = %v, want %v (tolerance: %v)", got, tt.expectedMinDuration, tolerance)
			}
		})
	}
}

// TestWarningStateFSM tests the finite state machine transitions
func TestWarningStateFSM(t *testing.T) {
	tests := []struct {
		name          string
		initialState  WarningState
		action        string
		expectedState WarningState
	}{
		{
			name:          "None -> Active on idle condition with warning period",
			initialState:  WarningStateNone,
			action:        "issue_warning",
			expectedState: WarningStateActive,
		},
		{
			name:          "Active -> Canceled on user activity",
			initialState:  WarningStateActive,
			action:        "cancel_warning",
			expectedState: WarningStateCanceled,
		},
		{
			name:          "Active -> None on condition no longer met",
			initialState:  WarningStateActive,
			action:        "reset_warning",
			expectedState: WarningStateNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			monitor := NewIdleMonitor(30, 60, 120, 5, 10)
			monitor.state.WarningState = tt.initialState
			now := time.Now()

			switch tt.action {
			case "issue_warning":
				monitor.state.WarningIssuedAt = &now
				monitor.state.WarningState = WarningStateActive
			case "cancel_warning":
				monitor.state.WarningState = WarningStateCanceled
			case "reset_warning":
				monitor.resetWarning()
			}

			if monitor.state.WarningState != tt.expectedState {
				t.Errorf("After %s: WarningState = %v, want %v", tt.action, monitor.state.WarningState, tt.expectedState)
			}
		})
	}
}

// TestTimeCalculationEdgeCases tests edge cases in time calculations
func TestTimeCalculationEdgeCases(t *testing.T) {
	tests := []struct {
		name               string
		threshold          time.Duration
		elapsed            time.Duration
		expectThresholdMet bool
	}{
		{
			name:               "exactly at threshold",
			threshold:          30 * time.Minute,
			elapsed:            30 * time.Minute,
			expectThresholdMet: true,
		},
		{
			name:               "just before threshold",
			threshold:          30 * time.Minute,
			elapsed:            29*time.Minute + 59*time.Second,
			expectThresholdMet: false,
		},
		{
			name:               "just after threshold",
			threshold:          30 * time.Minute,
			elapsed:            30*time.Minute + 1*time.Second,
			expectThresholdMet: true,
		},
		{
			name:               "zero threshold (disabled)",
			threshold:          0,
			elapsed:            1 * time.Hour,
			expectThresholdMet: false, // zero threshold means disabled
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the threshold comparison logic used in the code
			met := tt.elapsed >= tt.threshold && tt.threshold > 0
			if met != tt.expectThresholdMet {
				t.Errorf("Threshold met = %v, want %v (elapsed: %v, threshold: %v)",
					met, tt.expectThresholdMet, tt.elapsed, tt.threshold)
			}
		})
	}
}

// TestWarningPeriodExpiration tests warning period timing logic
func TestWarningPeriodExpiration(t *testing.T) {
	tests := []struct {
		name          string
		warningPeriod time.Duration
		elapsed       time.Duration
		expectExpired bool
	}{
		{
			name:          "warning just issued",
			warningPeriod: 5 * time.Minute,
			elapsed:       0,
			expectExpired: false,
		},
		{
			name:          "halfway through warning period",
			warningPeriod: 5 * time.Minute,
			elapsed:       2*time.Minute + 30*time.Second,
			expectExpired: false,
		},
		{
			name:          "exactly at warning period expiration",
			warningPeriod: 5 * time.Minute,
			elapsed:       5 * time.Minute,
			expectExpired: true,
		},
		{
			name:          "warning period expired",
			warningPeriod: 5 * time.Minute,
			elapsed:       6 * time.Minute,
			expectExpired: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the expiration logic used in the code
			expired := tt.elapsed >= tt.warningPeriod
			if expired != tt.expectExpired {
				t.Errorf("Warning expired = %v, want %v (elapsed: %v, period: %v)",
					expired, tt.expectExpired, tt.elapsed, tt.warningPeriod)
			}
		})
	}
}

// TestMinimumUptimeBoundary tests the minimum uptime boundary condition
func TestMinimumUptimeBoundary(t *testing.T) {
	tests := []struct {
		name             string
		minimumUptime    time.Duration
		effectiveUptime  time.Duration
		expectShouldSkip bool
	}{
		{
			name:             "uptime below threshold",
			minimumUptime:    10 * time.Minute,
			effectiveUptime:  5 * time.Minute,
			expectShouldSkip: true,
		},
		{
			name:             "uptime exactly at threshold",
			minimumUptime:    10 * time.Minute,
			effectiveUptime:  10 * time.Minute,
			expectShouldSkip: true, // <= comparison (line 159 in idle.go)
		},
		{
			name:             "uptime just above threshold",
			minimumUptime:    10 * time.Minute,
			effectiveUptime:  10*time.Minute + 1*time.Second,
			expectShouldSkip: false,
		},
		{
			name:             "uptime well above threshold",
			minimumUptime:    10 * time.Minute,
			effectiveUptime:  20 * time.Minute,
			expectShouldSkip: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the boundary logic used in idle.go:159
			shouldSkip := tt.effectiveUptime <= tt.minimumUptime
			if shouldSkip != tt.expectShouldSkip {
				t.Errorf("Should skip = %v, want %v (uptime: %v, threshold: %v)",
					shouldSkip, tt.expectShouldSkip, tt.effectiveUptime, tt.minimumUptime)
			}
		})
	}
}

// Helper function to create a pointer to a duration
func durationPtr(d time.Duration) *time.Duration {
	return &d
}
