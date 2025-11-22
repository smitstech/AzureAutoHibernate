//go:build windows

package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/smitstech/AzureAutoHibernate/internal/appinfo"
	"github.com/smitstech/AzureAutoHibernate/internal/azure"
	"github.com/smitstech/AzureAutoHibernate/internal/config"
	"github.com/smitstech/AzureAutoHibernate/internal/installer"
	"github.com/smitstech/AzureAutoHibernate/internal/logger"
	"github.com/smitstech/AzureAutoHibernate/internal/service"
	"github.com/smitstech/AzureAutoHibernate/internal/version"
	"golang.org/x/sys/windows/svc"
)

// options holds command-line flags
type options struct {
	configPath  string
	debugMode   bool
	install     bool
	uninstall   bool
	showVersion bool
}

// parseFlags parses command-line flags and returns options
func parseFlags() *options {
	opts := &options{}
	flag.StringVar(&opts.configPath, "config", "", "Path to configuration file (default: config.json in executable directory)")
	flag.BoolVar(&opts.debugMode, "debug", false, "Run in debug mode (console) instead of as a service")
	flag.BoolVar(&opts.install, "install", false, "Install the service")
	flag.BoolVar(&opts.uninstall, "uninstall", false, "Uninstall the service")
	flag.BoolVar(&opts.showVersion, "version", false, "Show version information")
	flag.Parse()
	return opts
}

func main() {
	// Setup basic console logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	opts := parseFlags()

	// Dispatch to appropriate mode
	switch {
	case opts.showVersion:
		fmt.Println(version.Short())
		os.Exit(0)
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
	if err := installer.Install(); err != nil {
		log.Fatalf("Failed to install service: %v", err)
	}
	log.Println("Service installed successfully")
}

// runUninstall handles service uninstallation
func runUninstall() {
	if err := installer.Uninstall(); err != nil {
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
