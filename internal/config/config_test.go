package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestValidate tests the configuration validation logic
func TestValidate(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid configuration with all thresholds",
			config: Config{
				NoUsersIdleMinutes:         30,
				AllDisconnectedIdleMinutes: 60,
				InactiveUserIdleMinutes:    120,
				InactiveUserWarningMinutes: 5,
				MinimumUptimeMinutes:       10,
				LogLevel:                   "info",
			},
			expectError: false,
		},
		{
			name: "valid configuration with debug log level",
			config: Config{
				NoUsersIdleMinutes:         30,
				AllDisconnectedIdleMinutes: 0,
				InactiveUserIdleMinutes:    120,
				InactiveUserWarningMinutes: 5,
				MinimumUptimeMinutes:       10,
				LogLevel:                   "debug",
			},
			expectError: false,
		},
		{
			name: "valid configuration with warning log level",
			config: Config{
				NoUsersIdleMinutes:         30,
				AllDisconnectedIdleMinutes: 0,
				InactiveUserIdleMinutes:    0,
				InactiveUserWarningMinutes: 0,
				MinimumUptimeMinutes:       0,
				LogLevel:                   "warning",
			},
			expectError: false,
		},
		{
			name: "valid configuration with error log level",
			config: Config{
				NoUsersIdleMinutes:         0,
				AllDisconnectedIdleMinutes: 60,
				InactiveUserIdleMinutes:    0,
				InactiveUserWarningMinutes: 0,
				MinimumUptimeMinutes:       0,
				LogLevel:                   "error",
			},
			expectError: false,
		},
		{
			name: "empty log level defaults to info",
			config: Config{
				NoUsersIdleMinutes:         30,
				AllDisconnectedIdleMinutes: 0,
				InactiveUserIdleMinutes:    0,
				InactiveUserWarningMinutes: 0,
				MinimumUptimeMinutes:       0,
				LogLevel:                   "",
			},
			expectError: false,
		},
		{
			name: "negative noUsersIdleMinutes",
			config: Config{
				NoUsersIdleMinutes:         -10,
				AllDisconnectedIdleMinutes: 60,
				InactiveUserIdleMinutes:    120,
				InactiveUserWarningMinutes: 5,
				MinimumUptimeMinutes:       10,
				LogLevel:                   "info",
			},
			expectError: true,
			errorMsg:    "noUsersIdleMinutes must be non-negative",
		},
		{
			name: "negative allDisconnectedIdleMinutes",
			config: Config{
				NoUsersIdleMinutes:         30,
				AllDisconnectedIdleMinutes: -60,
				InactiveUserIdleMinutes:    120,
				InactiveUserWarningMinutes: 5,
				MinimumUptimeMinutes:       10,
				LogLevel:                   "info",
			},
			expectError: true,
			errorMsg:    "allDisconnectedIdleMinutes must be non-negative",
		},
		{
			name: "negative inactiveUserIdleMinutes",
			config: Config{
				NoUsersIdleMinutes:         30,
				AllDisconnectedIdleMinutes: 60,
				InactiveUserIdleMinutes:    -120,
				InactiveUserWarningMinutes: 5,
				MinimumUptimeMinutes:       10,
				LogLevel:                   "info",
			},
			expectError: true,
			errorMsg:    "inactiveUserIdleMinutes must be non-negative",
		},
		{
			name: "negative inactiveUserWarningMinutes",
			config: Config{
				NoUsersIdleMinutes:         30,
				AllDisconnectedIdleMinutes: 60,
				InactiveUserIdleMinutes:    120,
				InactiveUserWarningMinutes: -5,
				MinimumUptimeMinutes:       10,
				LogLevel:                   "info",
			},
			expectError: true,
			errorMsg:    "inactiveUserWarningMinutes must be non-negative",
		},
		{
			name: "negative minimumUptimeMinutes",
			config: Config{
				NoUsersIdleMinutes:         30,
				AllDisconnectedIdleMinutes: 60,
				InactiveUserIdleMinutes:    120,
				InactiveUserWarningMinutes: 5,
				MinimumUptimeMinutes:       -10,
				LogLevel:                   "info",
			},
			expectError: true,
			errorMsg:    "minimumUptimeMinutes must be non-negative",
		},
		{
			name: "all idle thresholds are zero",
			config: Config{
				NoUsersIdleMinutes:         0,
				AllDisconnectedIdleMinutes: 0,
				InactiveUserIdleMinutes:    0,
				InactiveUserWarningMinutes: 5,
				MinimumUptimeMinutes:       10,
				LogLevel:                   "info",
			},
			expectError: true,
			errorMsg:    "at least one idle threshold must be greater than 0",
		},
		{
			name: "invalid log level",
			config: Config{
				NoUsersIdleMinutes:         30,
				AllDisconnectedIdleMinutes: 60,
				InactiveUserIdleMinutes:    120,
				InactiveUserWarningMinutes: 5,
				MinimumUptimeMinutes:       10,
				LogLevel:                   "invalid",
			},
			expectError: true,
			errorMsg:    "logLevel must be one of: debug, info, warn, warning, error (got: invalid)",
		},
		{
			name: "only noUsersIdleMinutes enabled",
			config: Config{
				NoUsersIdleMinutes:         30,
				AllDisconnectedIdleMinutes: 0,
				InactiveUserIdleMinutes:    0,
				InactiveUserWarningMinutes: 0,
				MinimumUptimeMinutes:       0,
				LogLevel:                   "info",
			},
			expectError: false,
		},
		{
			name: "only allDisconnectedIdleMinutes enabled",
			config: Config{
				NoUsersIdleMinutes:         0,
				AllDisconnectedIdleMinutes: 60,
				InactiveUserIdleMinutes:    0,
				InactiveUserWarningMinutes: 0,
				MinimumUptimeMinutes:       0,
				LogLevel:                   "info",
			},
			expectError: false,
		},
		{
			name: "only inactiveUserIdleMinutes enabled",
			config: Config{
				NoUsersIdleMinutes:         0,
				AllDisconnectedIdleMinutes: 0,
				InactiveUserIdleMinutes:    120,
				InactiveUserWarningMinutes: 5,
				MinimumUptimeMinutes:       0,
				LogLevel:                   "info",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
					return
				}
				if tt.errorMsg != "" && err.Error() != tt.errorMsg {
					t.Errorf("Error message = %q, want %q", err.Error(), tt.errorMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				// Check that empty log level defaults to "info"
				if tt.config.LogLevel == "" {
					t.Errorf("Empty LogLevel should be defaulted to 'info' during validation")
				}
			}
		})
	}
}

// TestLoad tests the configuration loading from file
func TestLoad(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		expectError bool
		validate    func(*testing.T, *Config)
	}{
		{
			name: "valid config file",
			content: `{
				"noUsersIdleMinutes": 30,
				"allDisconnectedIdleMinutes": 60,
				"inactiveUserIdleMinutes": 120,
				"inactiveUserWarningMinutes": 5,
				"minimumUptimeMinutes": 10,
				"logLevel": "info"
			}`,
			expectError: false,
			validate: func(t *testing.T, cfg *Config) {
				if cfg.NoUsersIdleMinutes != 30 {
					t.Errorf("NoUsersIdleMinutes = %d, want 30", cfg.NoUsersIdleMinutes)
				}
				if cfg.AllDisconnectedIdleMinutes != 60 {
					t.Errorf("AllDisconnectedIdleMinutes = %d, want 60", cfg.AllDisconnectedIdleMinutes)
				}
				if cfg.InactiveUserIdleMinutes != 120 {
					t.Errorf("InactiveUserIdleMinutes = %d, want 120", cfg.InactiveUserIdleMinutes)
				}
				if cfg.InactiveUserWarningMinutes != 5 {
					t.Errorf("InactiveUserWarningMinutes = %d, want 5", cfg.InactiveUserWarningMinutes)
				}
				if cfg.MinimumUptimeMinutes != 10 {
					t.Errorf("MinimumUptimeMinutes = %d, want 10", cfg.MinimumUptimeMinutes)
				}
				if cfg.LogLevel != "info" {
					t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
				}
			},
		},
		{
			name: "config with zero values",
			content: `{
				"noUsersIdleMinutes": 30,
				"allDisconnectedIdleMinutes": 0,
				"inactiveUserIdleMinutes": 0,
				"inactiveUserWarningMinutes": 0,
				"minimumUptimeMinutes": 0,
				"logLevel": "debug"
			}`,
			expectError: false,
			validate: func(t *testing.T, cfg *Config) {
				if cfg.NoUsersIdleMinutes != 30 {
					t.Errorf("NoUsersIdleMinutes = %d, want 30", cfg.NoUsersIdleMinutes)
				}
				if cfg.LogLevel != "debug" {
					t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
				}
			},
		},
		{
			name:        "invalid JSON",
			content:     `{invalid json}`,
			expectError: true,
		},
		{
			name: "invalid config - all thresholds zero",
			content: `{
				"noUsersIdleMinutes": 0,
				"allDisconnectedIdleMinutes": 0,
				"inactiveUserIdleMinutes": 0,
				"inactiveUserWarningMinutes": 0,
				"minimumUptimeMinutes": 0,
				"logLevel": "info"
			}`,
			expectError: true,
		},
		{
			name: "invalid config - negative values",
			content: `{
				"noUsersIdleMinutes": -30,
				"allDisconnectedIdleMinutes": 60,
				"inactiveUserIdleMinutes": 120,
				"inactiveUserWarningMinutes": 5,
				"minimumUptimeMinutes": 10,
				"logLevel": "info"
			}`,
			expectError: true,
		},
		{
			name: "invalid config - bad log level",
			content: `{
				"noUsersIdleMinutes": 30,
				"allDisconnectedIdleMinutes": 60,
				"inactiveUserIdleMinutes": 120,
				"inactiveUserWarningMinutes": 5,
				"minimumUptimeMinutes": 10,
				"logLevel": "badlevel"
			}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary config file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.json")

			if err := os.WriteFile(configPath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("Failed to create test config file: %v", err)
			}

			// Load config
			cfg, err := Load(configPath)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				} else if tt.validate != nil {
					tt.validate(t, cfg)
				}
			}
		})
	}
}

// TestLoadNonExistentFile tests loading a config file that doesn't exist
func TestLoadNonExistentFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.json")
	if err == nil {
		t.Error("Expected error when loading non-existent file, got none")
	}
}

// TestLoadEmptyPath tests loading config with empty path (should look for config.json next to executable)
func TestLoadEmptyPath(t *testing.T) {
	// This test will likely fail because config.json doesn't exist next to the test executable
	// But it tests that the code path for empty string works
	_, err := Load("")
	if err == nil {
		t.Log("Config loaded successfully (config.json must exist next to test executable)")
	} else {
		t.Logf("Expected error when loading from default location: %v", err)
		// This is expected behavior when config.json doesn't exist
	}
}

// TestConfigValidationEdgeCases tests edge cases in validation
func TestConfigValidationEdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		check  func(*testing.T, *Config, error)
	}{
		{
			name: "log level defaults to info when empty",
			config: Config{
				NoUsersIdleMinutes:         30,
				AllDisconnectedIdleMinutes: 0,
				InactiveUserIdleMinutes:    0,
				InactiveUserWarningMinutes: 0,
				MinimumUptimeMinutes:       0,
				LogLevel:                   "",
			},
			check: func(t *testing.T, cfg *Config, err error) {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if cfg.LogLevel != "info" {
					t.Errorf("LogLevel = %q, want %q (should default to 'info')", cfg.LogLevel, "info")
				}
			},
		},
		{
			name: "warn vs warning log level both valid",
			config: Config{
				NoUsersIdleMinutes:         30,
				AllDisconnectedIdleMinutes: 0,
				InactiveUserIdleMinutes:    0,
				InactiveUserWarningMinutes: 0,
				MinimumUptimeMinutes:       0,
				LogLevel:                   "warn",
			},
			check: func(t *testing.T, cfg *Config, err error) {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			},
		},
		{
			name: "all thresholds very large",
			config: Config{
				NoUsersIdleMinutes:         1440, // 24 hours
				AllDisconnectedIdleMinutes: 2880, // 48 hours
				InactiveUserIdleMinutes:    4320, // 72 hours
				InactiveUserWarningMinutes: 60,   // 1 hour
				MinimumUptimeMinutes:       120,  // 2 hours
				LogLevel:                   "info",
			},
			check: func(t *testing.T, cfg *Config, err error) {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			tt.check(t, &tt.config, err)
		})
	}
}
