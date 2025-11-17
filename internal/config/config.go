package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	NoUsersIdleMinutes         int    `json:"noUsersIdleMinutes"`
	AllDisconnectedIdleMinutes int    `json:"allDisconnectedIdleMinutes"`
	InactiveUserIdleMinutes    int    `json:"inactiveUserIdleMinutes"`
	InactiveUserWarningMinutes int    `json:"inactiveUserWarningMinutes"`
	MinimumUptimeMinutes       int    `json:"minimumUptimeMinutes"`
	LogLevel                   string `json:"logLevel"`
}

// Load reads configuration from the specified path
func Load(configPath string) (*Config, error) {
	// If no path specified, look for config.json in the same directory as the executable
	if configPath == "" {
		exePath, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("failed to get executable path: %w", err)
		}
		configPath = filepath.Join(filepath.Dir(exePath), "config.json")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.NoUsersIdleMinutes < 0 {
		return fmt.Errorf("noUsersIdleMinutes must be non-negative")
	}
	if c.AllDisconnectedIdleMinutes < 0 {
		return fmt.Errorf("allDisconnectedIdleMinutes must be non-negative")
	}
	if c.InactiveUserIdleMinutes < 0 {
		return fmt.Errorf("inactiveUserIdleMinutes must be non-negative")
	}
	if c.InactiveUserWarningMinutes < 0 {
		return fmt.Errorf("inactiveUserWarningMinutes must be non-negative")
	}
	if c.MinimumUptimeMinutes < 0 {
		return fmt.Errorf("minimumUptimeMinutes must be non-negative")
	}

	// Ensure at least one idle condition is enabled
	if c.NoUsersIdleMinutes == 0 && c.AllDisconnectedIdleMinutes == 0 && c.InactiveUserIdleMinutes == 0 {
		return fmt.Errorf("at least one idle threshold must be greater than 0")
	}

	// Validate log level
	validLogLevels := map[string]bool{
		"debug":   true,
		"info":    true,
		"warn":    true,
		"warning": true,
		"error":   true,
	}
	if c.LogLevel == "" {
		c.LogLevel = "info" // default to info if not specified
	}
	if !validLogLevels[c.LogLevel] {
		return fmt.Errorf("logLevel must be one of: debug, info, warn, warning, error (got: %s)", c.LogLevel)
	}

	return nil
}
