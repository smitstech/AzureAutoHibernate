//go:build windows

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/smitstech/AzureAutoHibernate/internal/appinfo"
	"github.com/smitstech/AzureAutoHibernate/internal/azure"
	"github.com/smitstech/AzureAutoHibernate/internal/config"
	"github.com/smitstech/AzureAutoHibernate/internal/logger"
	"github.com/smitstech/AzureAutoHibernate/internal/service"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
)

// options holds command-line flags
type options struct {
	configPath string
	debugMode  bool
	install    bool
	uninstall  bool
}

// parseFlags parses command-line flags and returns options
func parseFlags() *options {
	opts := &options{}
	flag.StringVar(&opts.configPath, "config", "", "Path to configuration file (default: config.json in executable directory)")
	flag.BoolVar(&opts.debugMode, "debug", false, "Run in debug mode (console) instead of as a service")
	flag.BoolVar(&opts.install, "install", false, "Install the service")
	flag.BoolVar(&opts.uninstall, "uninstall", false, "Uninstall the service")
	flag.Parse()
	return opts
}

func main() {
	// Setup basic console logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	opts := parseFlags()

	// Dispatch to appropriate mode
	switch {
	case opts.install:
		runInstall()
	case opts.uninstall:
		runUninstall()
	default:
		runServiceOrDebug(opts)
	}
}

// runInstall handles service installation
func runInstall() {
	if err := installService(); err != nil {
		log.Fatalf("Failed to install service: %v", err)
	}
	log.Println("Service installed successfully")
}

// runUninstall handles service uninstallation
func runUninstall() {
	if err := uninstallService(); err != nil {
		log.Fatalf("Failed to uninstall service: %v", err)
	}
	log.Println("Service uninstalled successfully")
}

// runServiceOrDebug runs the main service logic in either service or debug mode
func runServiceOrDebug(opts *options) {
	// Load configuration
	cfg, err := config.Load(opts.configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Determine if running as service or console
	isInteractive, err := svc.IsAnInteractiveSession()
	if err != nil {
		log.Fatalf("Failed to determine if running interactively: %v", err)
	}

	// If debug flag is set, force interactive mode
	if opts.debugMode {
		isInteractive = true
	}

	// Create appropriate logger
	logLevel := logger.ParseLogLevel(cfg.LogLevel)
	var appLogger logger.Logger

	if isInteractive {
		// Use console logger for debug mode
		appLogger = logger.NewConsoleLogger(logLevel)
	} else {
		// Use Windows Event Log for service mode
		appLogger, err = logger.NewEventLogger(appinfo.ServiceName, logLevel)
		if err != nil {
			log.Fatalf("Failed to open event log: %v", err)
		}
		defer appLogger.Close()
	}

	// Log startup
	appLogger.Info(logger.EventServiceStart, appinfo.Name+" service starting...")
	appLogger.Debugf(logger.EventConfigLoaded, "Log level set to: %s", logLevel.String())

	// Fetch VM metadata from Azure IMDS
	appLogger.Info(logger.EventConfigLoaded, "Fetching VM metadata from Azure IMDS...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	vmMetadata, err := azure.GetVMMetadata(ctx)
	if err != nil {
		appLogger.Errorf(logger.EventConfigError, "Failed to get VM metadata from IMDS: %v", err)
		log.Fatalf("Failed to get VM metadata from IMDS: %v\nThe service must run on an Azure VM with access to the Instance Metadata Service.", err)
	}

	appLogger.Infof(logger.EventConfigLoaded, "VM Info: Subscription=%s, ResourceGroup=%s, VMName=%s",
		vmMetadata.SubscriptionId, vmMetadata.ResourceGroup, vmMetadata.VMName)
	appLogger.Debugf(logger.EventConfigLoaded, "Config: NoUsers=%dm, AllDisconnected=%dm, InactiveUser=%dm, InactiveUserWarning=%dm",
		cfg.NoUsersIdleMinutes, cfg.AllDisconnectedIdleMinutes,
		cfg.InactiveUserIdleMinutes, cfg.InactiveUserWarningMinutes)

	// Run the service
	if err := service.Run(cfg, vmMetadata, appLogger, isInteractive); err != nil {
		appLogger.Errorf(logger.EventServiceStop, "Service failed: %v", err)
		log.Fatalf("Service failed: %v", err)
	}
}

// testAzureCapabilities tests the VM's Azure hibernation capabilities and displays results.
// Returns the test result or an error if critical requirements are not met.
func testAzureCapabilities(ctx context.Context) (*azure.HibernationCapabilityResult, error) {
	log.Println("")
	log.Println("=== Testing Azure Hibernation Capability ===")
	log.Println("Checking if this VM can be hibernated via Azure...")

	result := azure.TestHibernationCapability(ctx)

	// Display and validate IMDS availability
	if !result.IMDSAvailable {
		log.Println("")
		log.Println("❌ IMDS Check: FAILED")
		log.Printf("   Error: %v", result.IMDSError)
		log.Println("   This VM cannot access the Azure Instance Metadata Service.")
		log.Println("   Possible causes:")
		log.Println("   - Not running on Azure")
		log.Println("   - Network connectivity issues")
		log.Println("   - IMDS endpoint blocked by firewall")
		return nil, fmt.Errorf("IMDS not available: %w", result.IMDSError)
	}

	log.Println("✓ IMDS Check: PASSED")
	log.Printf("   VM: %s", result.VMMetadata.VMName)
	log.Printf("   Resource Group: %s", result.VMMetadata.ResourceGroup)
	log.Printf("   Subscription: %s", result.VMMetadata.SubscriptionId)

	// Display and validate managed identity
	if !result.TokenSuccess {
		log.Println("")
		log.Println("❌ Managed Identity Check: FAILED")
		log.Printf("   Error: %v", result.TokenError)
		log.Println("   The VM's Managed Identity is not properly configured.")
		log.Println("   Required actions:")
		log.Println("   1. Enable System-Assigned Managed Identity on this VM")
		log.Println("   2. Grant the identity 'Virtual Machine Contributor' role")
		log.Println("   3. Ensure the role is scoped to this VM or resource group")
		return nil, fmt.Errorf("managed identity not configured: %w", result.TokenError)
	}

	log.Println("✓ Managed Identity Check: PASSED")
	log.Println("   Successfully retrieved access token from IMDS")

	// Display and validate hibernation API access
	if result.HibernationAPIError != nil {
		log.Println("")
		log.Println("❌ VM Hibernation API Check: FAILED")
		log.Printf("   Error: %v", result.HibernationAPIError)
		log.Println("   Could not verify VM hibernation capability via Azure API.")
		log.Println("   Possible causes:")
		log.Println("   - Managed Identity lacks 'Virtual Machine Contributor' or 'Reader' role")
		log.Println("   - Role assignment not yet propagated (can take a few minutes)")
		log.Println("   - Network connectivity issues to Azure Management API")
		return nil, fmt.Errorf("hibernation API check failed: %w", result.HibernationAPIError)
	}

	// Display hibernation feature status (warning only, not fatal)
	if !result.HibernationEnabled {
		log.Println("")
		log.Println("❌ VM Hibernation Feature: DISABLED")
		log.Println("   Hibernation is not enabled on this VM.")
		log.Println("   Required actions:")
		log.Println("   1. Stop (deallocate) this VM in Azure Portal")
		log.Println("   2. Enable hibernation in VM settings")
		log.Println("   3. Ensure VM size supports hibernation (e.g., D-series, E-series)")
		log.Println("   4. Ensure OS disk is configured for hibernation")
		log.Println("   5. Restart the VM")
		log.Println("")
		log.Println("   Note: The service will be installed but hibernation will fail until this is fixed.")
		log.Println("   You may proceed with installation if you plan to enable hibernation later.")
	} else {
		log.Println("✓ VM Hibernation Feature: ENABLED")
		log.Println("   VM is configured for hibernation in Azure")
	}

	log.Println("")
	log.Println("✓ All prerequisite checks passed!")
	log.Println("  This VM can communicate with Azure and authenticate successfully.")

	return result, nil
}

// registerEventLogSource registers the Windows Event Log source for the service.
func registerEventLogSource(serviceName string) error {
	log.Println("")
	log.Println("=== Installing Service ===")

	err := eventlog.InstallAsEventCreate(serviceName, eventlog.Error|eventlog.Warning|eventlog.Info)
	if err != nil {
		// Check if the error is because the source already exists
		if strings.Contains(err.Error(), "registry key already exists") {
			log.Printf("Event log source '%s' already registered (skipping)", serviceName)
			return nil
		}
		return fmt.Errorf("failed to install event log source: %w", err)
	}

	log.Printf("Event log source '%s' registered successfully", serviceName)
	return nil
}

// displayInstallationInstructions shows the user how to complete service installation.
func displayInstallationInstructions(serviceName, exePath string) {
	log.Println("")
	log.Println("To complete the service installation, run as Administrator:")
	log.Printf(`sc create %s binPath= "%s" start= auto`, serviceName, exePath)
	log.Printf(`sc description %s "Monitors VM idleness and hibernates when idle"`, serviceName)
	log.Println("")
	log.Printf("Then start the service with: sc start %s", serviceName)
}

// installService orchestrates the service installation process.
// It tests Azure capabilities, registers the event log source, and provides installation instructions.
func installService() error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	// Check for notifier executable
	exeDir := filepath.Dir(exePath)
	notifierPath := filepath.Join(exeDir, appinfo.NotifierExeName)
	if _, err := os.Stat(notifierPath); err != nil {
		log.Println("")
		log.Println("⚠ WARNING: Notifier executable not found")
		log.Printf("   Expected location: %s", notifierPath)
		log.Println("   User notifications will not be sent.")
		log.Printf("   Please ensure both %s and %s", appinfo.MainExeName, appinfo.NotifierExeName)
		log.Println("   are in the same directory for full functionality.")
		log.Println("")
	} else {
		log.Println("✓ Notifier executable found")
		log.Printf("   Location: %s", notifierPath)
	}

	// Test Azure hibernation capabilities
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if _, err := testAzureCapabilities(ctx); err != nil {
		return err
	}

	// Register the event log source
	if err := registerEventLogSource(appinfo.ServiceName); err != nil {
		return err
	}

	// Display installation instructions
	displayInstallationInstructions(appinfo.ServiceName, exePath)

	return nil
}

func uninstallService() error {
	log.Println("To uninstall the service, first run as Administrator:")
	log.Printf("sc stop %s", appinfo.ServiceName)
	log.Printf("sc delete %s", appinfo.ServiceName)

	// Remove the event log source
	err := eventlog.Remove(appinfo.ServiceName)
	if err != nil {
		return fmt.Errorf("failed to remove event log source: %w", err)
	}
	log.Println("")
	log.Printf("Event log source '%s' removed successfully", appinfo.ServiceName)

	return nil
}
