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

	// Auto-update settings
	AutoUpdate            bool `json:"autoUpdate"`            // Enable automatic updates (default: false)
	UpdateCheckIntervalHr int  `json:"updateCheckIntervalHr"` // Hours between update checks (default: 24)

	// Service Principal auth (optional). If all three are set, auth uses
	// login.microsoftonline.com (public internet) instead of the VM's managed
	// identity via IMDS — useful when IMDS (169.254.169.254) is unreachable,
	// e.g. when a VPN/exit-node intercepts link-local traffic.
	TenantID     string `json:"tenantId"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`

	// Optional VM identity override. Required when IMDS is unreachable and
	// Service Principal auth is used; ignored otherwise.
	SubscriptionID string `json:"subscriptionId"`
	ResourceGroup  string `json:"resourceGroup"`
	VMNameOverride string `json:"vmName"`
}

// HasVMIdentityOverride reports whether config fully specifies the VM identity
func (c *Config) HasVMIdentityOverride() bool {
	return c.SubscriptionID != "" && c.ResourceGroup != "" && c.VMNameOverride != ""
}

// UseServicePrincipal reports whether Service Principal auth is configured
func (c *Config) UseServicePrincipal() bool {
	return c.TenantID != "" && c.ClientID != "" && c.ClientSecret != ""
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

	// Default update check interval to 24 hours if not specified or invalid
	if c.UpdateCheckIntervalHr <= 0 {
		c.UpdateCheckIntervalHr = 24
	}

	// Service Principal: all three fields must be provided together
	spSet := 0
	for _, v := range []string{c.TenantID, c.ClientID, c.ClientSecret} {
		if v != "" {
			spSet++
		}
	}
	if spSet != 0 && spSet != 3 {
		return fmt.Errorf("service principal auth requires tenantId, clientId AND clientSecret (all or none)")
	}

	return nil
}
