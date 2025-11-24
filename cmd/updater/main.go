//go:build windows

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

func main() {
	// Parse command-line flags
	serviceName := flag.String("service-name", "", "Name of the Windows service to update")
	exePath := flag.String("exe-path", "", "Path to the current executable")
	updateDir := flag.String("update-dir", "", "Directory containing the new files")
	flag.Parse()

	// Setup logging to a file
	logFile, err := os.OpenFile(filepath.Join(os.TempDir(), "AzureAutoHibernate.Updater.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		log.SetOutput(logFile)
		defer logFile.Close()
	}

	log.Printf("Updater started: service=%s, exe=%s, updateDir=%s", *serviceName, *exePath, *updateDir)

	if *serviceName == "" || *exePath == "" || *updateDir == "" {
		log.Fatal("Missing required arguments")
	}

	// Attempt to stop the service (retry in case of transient errors)
	maxStopAttempts := 3
	var stopErr error
	for attempt := 1; attempt <= maxStopAttempts; attempt++ {
		log.Printf("Sending stop command to service (attempt %d/%d)", attempt, maxStopAttempts)
		stopErr = stopService(*serviceName)
		if stopErr == nil {
			break
		}
		log.Printf("Failed to send stop command: %v", stopErr)
		if attempt < maxStopAttempts {
			time.Sleep(5 * time.Second)
		}
	}

	if stopErr != nil {
		log.Fatalf("Failed to stop service after %d attempts: %v - cannot proceed with update", maxStopAttempts, stopErr)
	}

	// Wait for the service to fully stop (with generous timeout and progress logging)
	waitTimeout := 10 * time.Minute
	log.Printf("Waiting for service to stop (timeout: %v)...", waitTimeout)
	if err := waitForServiceStop(*serviceName, waitTimeout); err != nil {
		log.Fatalf("Service failed to stop: %v - cannot proceed with update", err)
	}

	log.Println("Service stopped successfully")

	// Additional wait to ensure files are released
	time.Sleep(2 * time.Second)

	// Apply the update
	if err := applyUpdate(*exePath, *updateDir); err != nil {
		log.Fatalf("Failed to apply update: %v", err)
	}

	log.Println("Update applied successfully")

	// Start the service
	if err := startService(*serviceName); err != nil {
		log.Printf("Warning: failed to start service: %v", err)
		// Don't fatal - the user can start it manually
	} else {
		log.Println("Service started successfully")
	}

	// Cleanup update directory
	os.RemoveAll(*updateDir)
	log.Println("Cleanup complete, updater exiting")
}

// waitForServiceStop waits for the service to enter the stopped state
func waitForServiceStop(serviceName string, timeout time.Duration) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("failed to open service: %w", err)
	}
	defer s.Close()

	deadline := time.Now().Add(timeout)
	checkCount := 0
	for time.Now().Before(deadline) {
		status, err := s.Query()
		if err != nil {
			return fmt.Errorf("failed to query service status: %w", err)
		}

		if status.State == svc.Stopped {
			log.Println("Service is stopped")
			return nil
		}

		checkCount++
		// Log every 10 seconds to show progress
		if checkCount%10 == 0 {
			elapsed := time.Since(time.Now().Add(-timeout).Add(time.Until(deadline)))
			log.Printf("Still waiting for service to stop (state: %d, elapsed: %v)", status.State, elapsed.Round(time.Second))
		}
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("timeout waiting for service to stop")
}

// applyUpdate copies new files from updateDir to the executable directory
func applyUpdate(exePath, updateDir string) error {
	exeDir := filepath.Dir(exePath)

	// Find files in update directory
	entries, err := os.ReadDir(updateDir)
	if err != nil {
		return fmt.Errorf("failed to read update directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Skip config.json - we'll handle it separately with merge
		if entry.Name() == "config.json" {
			log.Println("Skipping config.json (will be merged separately)")
			continue
		}

		srcPath := filepath.Join(updateDir, entry.Name())
		dstPath := filepath.Join(exeDir, entry.Name())

		// Backup old file by renaming
		if _, err := os.Stat(dstPath); err == nil {
			backupPath := dstPath + ".old"
			os.Remove(backupPath) // Remove any existing backup
			if err := os.Rename(dstPath, backupPath); err != nil {
				log.Printf("Warning: failed to backup %s: %v", entry.Name(), err)
				// Try to continue anyway
			}
		}

		// Copy new file
		if err := copyFile(srcPath, dstPath); err != nil {
			return fmt.Errorf("failed to copy %s: %w", entry.Name(), err)
		}

		log.Printf("Updated: %s", entry.Name())
	}

	// Now merge config.json (preserving user settings)
	log.Println("Merging config.json...")
	if err := mergeConfigFile(exeDir, updateDir); err != nil {
		return fmt.Errorf("failed to merge config: %w", err)
	}

	return nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	return dstFile.Sync()
}

// stopService sends a stop command to the Windows service
func stopService(serviceName string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("failed to open service: %w", err)
	}
	defer s.Close()

	status, err := s.Control(svc.Stop)
	if err != nil {
		return fmt.Errorf("failed to send stop command: %w", err)
	}

	log.Printf("Stop command sent, service state: %d", status.State)
	return nil
}

// mergeConfigFile merges the new config with the existing one, preserving user settings
func mergeConfigFile(exeDir, updateDir string) error {
	configName := "config.json"
	existingPath := filepath.Join(exeDir, configName)
	newPath := filepath.Join(updateDir, configName)

	// Check if new config exists in update
	if _, err := os.Stat(newPath); os.IsNotExist(err) {
		log.Println("No config.json in update, skipping config merge")
		return nil
	}

	// Load new config
	newData, err := os.ReadFile(newPath)
	if err != nil {
		return fmt.Errorf("failed to read new config: %w", err)
	}

	var newConfig map[string]interface{}
	if err := json.Unmarshal(newData, &newConfig); err != nil {
		return fmt.Errorf("failed to parse new config: %w", err)
	}

	// Check if existing config exists
	var existingConfig map[string]interface{}
	if existingData, err := os.ReadFile(existingPath); err == nil {
		// Existing config found, merge it
		if err := json.Unmarshal(existingData, &existingConfig); err != nil {
			log.Printf("Warning: failed to parse existing config: %v - using new config", err)
			existingConfig = nil
		}
	} else if os.IsNotExist(err) {
		log.Println("No existing config.json found, using new config")
		existingConfig = nil
	} else {
		return fmt.Errorf("failed to read existing config: %w", err)
	}

	// Merge configs: keep existing values, add new keys
	var mergedConfig map[string]interface{}
	var addedKeys []string

	if existingConfig == nil {
		// No existing config, use new one entirely
		mergedConfig = newConfig
		log.Println("Using new config.json (no existing config found)")
	} else {
		// Merge: start with existing, add new keys
		mergedConfig = existingConfig
		for key, newValue := range newConfig {
			if _, exists := existingConfig[key]; !exists {
				mergedConfig[key] = newValue
				addedKeys = append(addedKeys, key)
			}
		}

		if len(addedKeys) > 0 {
			log.Printf("Added new config fields: %v", addedKeys)
		} else {
			log.Println("No new config fields to add")
		}
	}

	// Write merged config back
	mergedData, err := json.MarshalIndent(mergedConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal merged config: %w", err)
	}

	// Backup existing config
	if existingConfig != nil {
		backupPath := existingPath + ".old"
		os.Remove(backupPath)
		if err := os.Rename(existingPath, backupPath); err != nil {
			log.Printf("Warning: failed to backup config: %v", err)
		}
	}

	if err := os.WriteFile(existingPath, mergedData, 0644); err != nil {
		return fmt.Errorf("failed to write merged config: %w", err)
	}

	log.Println("Config merge completed successfully")
	return nil
}

// startService starts the Windows service
func startService(serviceName string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("failed to open service: %w", err)
	}
	defer s.Close()

	if err := s.Start(); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	return nil
}
