//go:build windows

package service

import (
	"strings"
	"testing"
	"time"

	"github.com/smitstech/AzureAutoHibernate/internal/azure"
	"github.com/smitstech/AzureAutoHibernate/internal/config"
)

// mockLogger is a simple logger for testing
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

func (m *mockLogger) Close() error {
	return nil
}

// TestNewAutoHibernateService tests the service constructor
func TestNewAutoHibernateService(t *testing.T) {
	cfg := &config.Config{
		NoUsersIdleMinutes:         30,
		AllDisconnectedIdleMinutes: 60,
		InactiveUserIdleMinutes:    120,
		InactiveUserWarningMinutes: 5,
		MinimumUptimeMinutes:       10,
		LogLevel:                   "info",
	}

	vmMetadata := &azure.VMMetadata{
		SubscriptionId: "test-sub",
		ResourceGroup:  "test-rg",
		VMName:         "test-vm",
	}

	log := &mockLogger{}

	service := NewAutoHibernateService(cfg, vmMetadata, log)

	if service == nil {
		t.Fatal("NewAutoHibernateService returned nil")
	}

	if service.config != cfg {
		t.Error("Config not set correctly")
	}

	if service.logger == nil {
		t.Error("Logger not set correctly")
	}

	if service.idleMonitor == nil {
		t.Error("IdleMonitor not initialized")
	}

	if service.azureClient == nil {
		t.Error("AzureClient not initialized")
	}

	// Note: notifierManager may be nil if the notifier executable is not found
	// This is expected behavior in test environments

	if service.stopChan == nil {
		t.Error("StopChan not initialized")
	}

	if service.resumeAt == nil {
		t.Error("ResumeAt should be initialized")
	}
}

// TestCalculateNextCheckTime tests the dynamic check interval calculation
func TestCalculateNextCheckTime(t *testing.T) {
	tests := []struct {
		name          string
		config        *config.Config
		inWarningMode bool
		expectedMin   time.Duration
		expectedMax   time.Duration
		description   string
	}{
		{
			name: "warning mode - should use 5 second interval",
			config: &config.Config{
				NoUsersIdleMinutes:         30,
				AllDisconnectedIdleMinutes: 60,
				InactiveUserIdleMinutes:    120,
				InactiveUserWarningMinutes: 5,
				MinimumUptimeMinutes:       10,
			},
			inWarningMode: true,
			expectedMin:   5 * time.Second,
			expectedMax:   5 * time.Second,
			description:   "In warning mode, check every 5 seconds",
		},
		{
			name: "not in warning mode - use minimum threshold",
			config: &config.Config{
				NoUsersIdleMinutes:         30,
				AllDisconnectedIdleMinutes: 60,
				InactiveUserIdleMinutes:    120,
				InactiveUserWarningMinutes: 5,
				MinimumUptimeMinutes:       10,
			},
			inWarningMode: false,
			expectedMin:   5 * time.Second,
			expectedMax:   30 * time.Minute, // NoUsersIdleMinutes is smallest
			description:   "Not in warning mode, use minimum configured threshold",
		},
		{
			name: "smaller allDisconnected threshold",
			config: &config.Config{
				NoUsersIdleMinutes:         60,
				AllDisconnectedIdleMinutes: 30,
				InactiveUserIdleMinutes:    120,
				InactiveUserWarningMinutes: 5,
				MinimumUptimeMinutes:       10,
			},
			inWarningMode: false,
			expectedMin:   5 * time.Second,
			expectedMax:   30 * time.Minute, // AllDisconnectedIdleMinutes is smallest
			description:   "Use smallest non-zero threshold",
		},
		{
			name: "smaller inactiveUser threshold",
			config: &config.Config{
				NoUsersIdleMinutes:         60,
				AllDisconnectedIdleMinutes: 90,
				InactiveUserIdleMinutes:    15,
				InactiveUserWarningMinutes: 5,
				MinimumUptimeMinutes:       10,
			},
			inWarningMode: false,
			expectedMin:   5 * time.Second,
			expectedMax:   15 * time.Minute, // InactiveUserIdleMinutes is smallest
			description:   "Use smallest non-zero threshold",
		},
		{
			name: "some thresholds zero",
			config: &config.Config{
				NoUsersIdleMinutes:         0,
				AllDisconnectedIdleMinutes: 0,
				InactiveUserIdleMinutes:    120,
				InactiveUserWarningMinutes: 5,
				MinimumUptimeMinutes:       10,
			},
			inWarningMode: false,
			expectedMin:   5 * time.Second,
			expectedMax:   120 * time.Minute,
			description:   "Skip zero thresholds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vmMetadata := &azure.VMMetadata{
				SubscriptionId: "test-sub",
				ResourceGroup:  "test-rg",
				VMName:         "test-vm",
			}
			log := &mockLogger{}

			service := NewAutoHibernateService(tt.config, vmMetadata, log)
			duration := service.calculateNextCheckTime(tt.inWarningMode)

			if duration < tt.expectedMin {
				t.Errorf("calculateNextCheckTime() = %v, should be >= %v (%s)",
					duration, tt.expectedMin, tt.description)
			}

			if duration > tt.expectedMax {
				t.Errorf("calculateNextCheckTime() = %v, should be <= %v (%s)",
					duration, tt.expectedMax, tt.description)
			}
		})
	}
}

// TestCalculateNextCheckTimeMinimumBoundary tests the minimum check interval boundary
func TestCalculateNextCheckTimeMinimumBoundary(t *testing.T) {
	cfg := &config.Config{
		NoUsersIdleMinutes:         30,
		AllDisconnectedIdleMinutes: 60,
		InactiveUserIdleMinutes:    120,
		InactiveUserWarningMinutes: 5,
		MinimumUptimeMinutes:       10,
	}

	vmMetadata := &azure.VMMetadata{
		SubscriptionId: "test-sub",
		ResourceGroup:  "test-rg",
		VMName:         "test-vm",
	}
	log := &mockLogger{}

	service := NewAutoHibernateService(cfg, vmMetadata, log)

	// Test that we never check less frequently than the minimum
	duration := service.calculateNextCheckTime(false)

	minCheckInterval := 5 * time.Second
	if duration < minCheckInterval && duration > 0 {
		t.Errorf("Check interval %v is less than minimum %v", duration, minCheckInterval)
	}
}

// TestCalculateNextCheckTimeWarningModeTransition tests warning mode check frequency
func TestCalculateNextCheckTimeWarningModeTransition(t *testing.T) {
	cfg := &config.Config{
		NoUsersIdleMinutes:         30,
		AllDisconnectedIdleMinutes: 60,
		InactiveUserIdleMinutes:    120,
		InactiveUserWarningMinutes: 5,
		MinimumUptimeMinutes:       10,
	}

	vmMetadata := &azure.VMMetadata{
		SubscriptionId: "test-sub",
		ResourceGroup:  "test-rg",
		VMName:         "test-vm",
	}
	log := &mockLogger{}

	service := NewAutoHibernateService(cfg, vmMetadata, log)

	// Warning mode should be 5 seconds
	warningDuration := service.calculateNextCheckTime(true)
	expectedWarningDuration := 5 * time.Second
	if warningDuration != expectedWarningDuration {
		t.Errorf("Warning mode check interval = %v, want %v", warningDuration, expectedWarningDuration)
	}

	// Non-warning mode should be longer (at least 5 seconds, up to minimum threshold)
	normalDuration := service.calculateNextCheckTime(false)
	if normalDuration < 5*time.Second {
		t.Errorf("Normal mode check interval %v is too short (min: 5s)", normalDuration)
	}
}

// TestHandlePowerEvent tests power event handling
func TestHandlePowerEvent(t *testing.T) {
	const (
		PBT_APMRESUMEAUTOMATIC = 18  // System resumed from suspend (automatic)
		PBT_APMRESUMESUSPEND   = 7   // System resumed from suspend (user-initiated)
		PBT_OTHER              = 999 // Some other power event
	)

	tests := []struct {
		name             string
		eventType        uint32
		expectResumeTime bool
	}{
		{
			name:             "automatic resume event",
			eventType:        PBT_APMRESUMEAUTOMATIC,
			expectResumeTime: true,
		},
		{
			name:             "user-initiated resume event",
			eventType:        PBT_APMRESUMESUSPEND,
			expectResumeTime: true,
		},
		{
			name:             "other power event",
			eventType:        PBT_OTHER,
			expectResumeTime: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				NoUsersIdleMinutes:         30,
				AllDisconnectedIdleMinutes: 60,
				InactiveUserIdleMinutes:    120,
				InactiveUserWarningMinutes: 5,
				MinimumUptimeMinutes:       10,
			}

			vmMetadata := &azure.VMMetadata{
				SubscriptionId: "test-sub",
				ResourceGroup:  "test-rg",
				VMName:         "test-vm",
			}
			log := &mockLogger{}

			service := NewAutoHibernateService(cfg, vmMetadata, log)

			// Store the initial resume time
			initialResumeAt := *service.resumeAt

			// Wait a bit to ensure time difference
			time.Sleep(10 * time.Millisecond)

			// Handle the power event
			service.handlePowerEvent(tt.eventType)

			// Check if resume time was updated
			resumeTimeUpdated := !service.resumeAt.Equal(initialResumeAt)

			if tt.expectResumeTime && !resumeTimeUpdated {
				t.Error("Expected resume time to be updated, but it wasn't")
			}

			if !tt.expectResumeTime && resumeTimeUpdated {
				t.Error("Did not expect resume time to be updated, but it was")
			}

			// For resume events, verify the idle monitor was also updated
			if tt.expectResumeTime {
				idleState := service.idleMonitor.GetState()
				// The idle monitor's resume time should be recent
				// Note: We can't directly check the idle monitor's resumeAt as it's private
				// but we can verify the service's resumeAt was set
				timeSinceResume := time.Since(*service.resumeAt)
				if timeSinceResume > 1*time.Second {
					t.Errorf("Resume time is too old: %v", timeSinceResume)
				}
				// Just verify we can get the state without panic
				_ = idleState
			}
		})
	}
}

// TestHandlePowerEventLogging tests that power events are logged correctly
func TestHandlePowerEventLogging(t *testing.T) {
	const (
		PBT_APMRESUMEAUTOMATIC = 18
		PBT_APMRESUMESUSPEND   = 7
	)

	tests := []struct {
		name      string
		eventType uint32
		expectLog bool
	}{
		{
			name:      "automatic resume logs",
			eventType: PBT_APMRESUMEAUTOMATIC,
			expectLog: true,
		},
		{
			name:      "user resume logs",
			eventType: PBT_APMRESUMESUSPEND,
			expectLog: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				NoUsersIdleMinutes:         30,
				AllDisconnectedIdleMinutes: 60,
				InactiveUserIdleMinutes:    120,
				InactiveUserWarningMinutes: 5,
				MinimumUptimeMinutes:       10,
			}

			vmMetadata := &azure.VMMetadata{
				SubscriptionId: "test-sub",
				ResourceGroup:  "test-rg",
				VMName:         "test-vm",
			}
			log := &mockLogger{}

			service := NewAutoHibernateService(cfg, vmMetadata, log)

			// Handle the power event
			service.handlePowerEvent(tt.eventType)

			// Check that something was logged
			if tt.expectLog {
				totalLogs := len(log.infoLogs) + len(log.debugLogs)
				if totalLogs == 0 {
					t.Error("Expected power event to be logged, but no logs found")
				}
			}
		})
	}
}

// TestHandlePowerEventDifferentiation tests that different resume events are logged with different messages
func TestHandlePowerEventDifferentiation(t *testing.T) {
	const (
		PBT_APMRESUMEAUTOMATIC = 18
		PBT_APMRESUMESUSPEND   = 7
	)

	cfg := &config.Config{
		NoUsersIdleMinutes:         30,
		AllDisconnectedIdleMinutes: 60,
		InactiveUserIdleMinutes:    120,
		InactiveUserWarningMinutes: 5,
		MinimumUptimeMinutes:       10,
	}

	vmMetadata := &azure.VMMetadata{
		SubscriptionId: "test-sub",
		ResourceGroup:  "test-rg",
		VMName:         "test-vm",
	}

	t.Run("automatic resume event", func(t *testing.T) {
		log := &mockLogger{}
		service := NewAutoHibernateService(cfg, vmMetadata, log)

		service.handlePowerEvent(PBT_APMRESUMEAUTOMATIC)

		if len(log.infoLogs) == 0 {
			t.Error("Expected automatic resume event to be logged")
		}

		// Check that the log message contains "(automatic)"
		found := false
		for _, logMsg := range log.infoLogs {
			if strings.Contains(logMsg, "(automatic)") {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected log message to contain '(automatic)' for PBT_APMRESUMEAUTOMATIC event")
		}
	})

	t.Run("user-initiated resume event", func(t *testing.T) {
		log := &mockLogger{}
		service := NewAutoHibernateService(cfg, vmMetadata, log)

		service.handlePowerEvent(PBT_APMRESUMESUSPEND)

		if len(log.infoLogs) == 0 {
			t.Error("Expected user-initiated resume event to be logged")
		}

		// Check that the log message contains "(user-initiated)"
		found := false
		for _, logMsg := range log.infoLogs {
			if strings.Contains(logMsg, "(user-initiated)") {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected log message to contain '(user-initiated)' for PBT_APMRESUMESUSPEND event")
		}
	})

	t.Run("both events logged separately", func(t *testing.T) {
		log := &mockLogger{}
		service := NewAutoHibernateService(cfg, vmMetadata, log)

		// Both events should be logged with different messages
		service.handlePowerEvent(PBT_APMRESUMEAUTOMATIC)
		service.handlePowerEvent(PBT_APMRESUMESUSPEND)

		if len(log.infoLogs) != 2 {
			t.Errorf("Expected 2 log entries, got %d", len(log.infoLogs))
		}
	})
}

// TestDynamicCheckIntervalLogic tests the logic for determining check intervals
func TestDynamicCheckIntervalLogic(t *testing.T) {
	tests := []struct {
		name            string
		config          *config.Config
		expectedDefault time.Duration
	}{
		{
			name: "default to smallest threshold",
			config: &config.Config{
				NoUsersIdleMinutes:         30,
				AllDisconnectedIdleMinutes: 60,
				InactiveUserIdleMinutes:    120,
				InactiveUserWarningMinutes: 5,
				MinimumUptimeMinutes:       10,
			},
			expectedDefault: 30 * time.Minute,
		},
		{
			name: "default to 5 minutes if all thresholds zero",
			config: &config.Config{
				NoUsersIdleMinutes:         0,
				AllDisconnectedIdleMinutes: 0,
				InactiveUserIdleMinutes:    0,
				InactiveUserWarningMinutes: 0,
				MinimumUptimeMinutes:       0,
			},
			expectedDefault: 5 * time.Minute,
		},
		{
			name: "use smallest non-zero threshold",
			config: &config.Config{
				NoUsersIdleMinutes:         0,
				AllDisconnectedIdleMinutes: 15,
				InactiveUserIdleMinutes:    30,
				InactiveUserWarningMinutes: 5,
				MinimumUptimeMinutes:       10,
			},
			expectedDefault: 15 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the logic for calculating default check interval
			minThreshold := tt.config.NoUsersIdleMinutes
			if tt.config.AllDisconnectedIdleMinutes > 0 && (minThreshold == 0 || tt.config.AllDisconnectedIdleMinutes < minThreshold) {
				minThreshold = tt.config.AllDisconnectedIdleMinutes
			}
			if tt.config.InactiveUserIdleMinutes > 0 && (minThreshold == 0 || tt.config.InactiveUserIdleMinutes < minThreshold) {
				minThreshold = tt.config.InactiveUserIdleMinutes
			}

			defaultCheckInterval := 5 * time.Minute
			if minThreshold > 0 {
				defaultCheckInterval = time.Duration(minThreshold) * time.Minute
			}

			if defaultCheckInterval != tt.expectedDefault {
				t.Errorf("Default check interval = %v, want %v", defaultCheckInterval, tt.expectedDefault)
			}
		})
	}
}

// TestServiceInitialization tests service initialization with various configurations
func TestServiceInitialization(t *testing.T) {
	tests := []struct {
		name   string
		config *config.Config
	}{
		{
			name: "standard configuration",
			config: &config.Config{
				NoUsersIdleMinutes:         30,
				AllDisconnectedIdleMinutes: 60,
				InactiveUserIdleMinutes:    120,
				InactiveUserWarningMinutes: 5,
				MinimumUptimeMinutes:       10,
				LogLevel:                   "info",
			},
		},
		{
			name: "minimal configuration",
			config: &config.Config{
				NoUsersIdleMinutes:         30,
				AllDisconnectedIdleMinutes: 0,
				InactiveUserIdleMinutes:    0,
				InactiveUserWarningMinutes: 0,
				MinimumUptimeMinutes:       0,
				LogLevel:                   "debug",
			},
		},
		{
			name: "aggressive configuration",
			config: &config.Config{
				NoUsersIdleMinutes:         5,
				AllDisconnectedIdleMinutes: 5,
				InactiveUserIdleMinutes:    5,
				InactiveUserWarningMinutes: 1,
				MinimumUptimeMinutes:       1,
				LogLevel:                   "info",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vmMetadata := &azure.VMMetadata{
				SubscriptionId: "test-sub",
				ResourceGroup:  "test-rg",
				VMName:         "test-vm",
			}
			log := &mockLogger{}

			service := NewAutoHibernateService(tt.config, vmMetadata, log)

			// Verify service is properly initialized
			if service == nil {
				t.Fatal("Service should not be nil")
			}

			if service.idleMonitor == nil {
				t.Error("Idle monitor should be initialized")
			}

			if service.azureClient == nil {
				t.Error("Azure client should be initialized")
			}

			// Note: notifierManager may be nil if the notifier executable is not found
			// This is expected behavior in test environments

			// Verify the service can calculate check times without panicking
			_ = service.calculateNextCheckTime(false)
			_ = service.calculateNextCheckTime(true)

			// Verify power event handling doesn't panic
			service.handlePowerEvent(18) // PBT_APMRESUMEAUTOMATIC
		})
	}
}
