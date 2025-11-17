//go:build windows

package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/smitstech/AzureAutoHibernate/internal/appinfo"
	"github.com/smitstech/AzureAutoHibernate/internal/notifier"
	"golang.org/x/sys/windows"
)

func main() {
	// Parse command-line flags
	sessionID := flag.Int("session", 0, "Session ID (0 for auto-detect)")
	flag.Parse()

	// Create logger
	logger, err := notifier.NewFileLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	logger.Info(appinfo.Name + " Notifier starting")

	// Get session ID if not provided
	if *sessionID == 0 {
		sid, err := getCurrentSessionID()
		if err != nil {
			logger.Error(fmt.Sprintf("Failed to get session ID: %v", err))
			os.Exit(1)
		}
		*sessionID = int(sid)
		logger.Info(fmt.Sprintf("Detected session ID: %d", *sessionID))
	}

	// Create UI handler
	ui := notifier.NewUI(logger)

	// Create pipe client
	client := notifier.NewPipeClient(*sessionID, ui, logger)

	// Start listening for commands
	err = client.Start()
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to start pipe client: %v", err))
		os.Exit(1)
	}

	logger.Info("Notifier started successfully")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan

	logger.Info("Shutting down notifier")
	client.Stop()
}

// getCurrentSessionID gets the current session ID
func getCurrentSessionID() (uint32, error) {
	var sessionID uint32
	err := windows.ProcessIdToSessionId(uint32(os.Getpid()), &sessionID)
	if err != nil {
		return 0, fmt.Errorf("failed to get session ID: %w", err)
	}
	return sessionID, nil
}
