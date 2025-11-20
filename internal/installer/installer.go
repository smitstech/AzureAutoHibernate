//go:build windows

package installer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/smitstech/AzureAutoHibernate/internal/appinfo"
	"github.com/smitstech/AzureAutoHibernate/internal/azure"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

// isAdmin checks if the current process is running with administrator privileges.
func isAdmin() (bool, error) {
	var sid *windows.SID

	// Get the SID for the Administrators group
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid)
	if err != nil {
		return false, fmt.Errorf("failed to get SID: %w", err)
	}
	defer windows.FreeSid(sid)

	// Check if the current process token is a member of the Administrators group
	token := windows.Token(0)
	member, err := token.IsMember(sid)
	if err != nil {
		return false, fmt.Errorf("failed to check token membership: %w", err)
	}

	return member, nil
}

// registerEventLogSource registers the Windows Event Log source for the service.
func registerEventLogSource(serviceName string) error {
	err := eventlog.InstallAsEventCreate(serviceName, eventlog.Error|eventlog.Warning|eventlog.Info)
	if err != nil {
		// Check if the error is because the source already exists
		if strings.Contains(err.Error(), "registry key already exists") {
			fmt.Printf("  [OK] Event log source '%s' already registered (skipping)\n", serviceName)
			return nil
		}
		return fmt.Errorf("failed to install event log source: %w", err)
	}

	fmt.Printf("  [OK] Event log source '%s' registered successfully\n", serviceName)
	return nil
}

// testAzureCapabilities tests the VM's Azure hibernation capabilities and displays results.
// Returns the test result or an error if critical requirements are not met.
func testAzureCapabilities(ctx context.Context) (*azure.HibernationCapabilityResult, error) {
	fmt.Println("")
	fmt.Println("=== Testing Azure Hibernation Capability ===")
	fmt.Println("Checking if this VM can be hibernated via Azure...")

	result := azure.TestHibernationCapability(ctx)

	// Display and validate IMDS availability
	if !result.IMDSAvailable {
		fmt.Println("")
		fmt.Println("[FAILED] IMDS Check")
		fmt.Printf("  Error: %v\n", result.IMDSError)
		fmt.Println("  This VM cannot access the Azure Instance Metadata Service.")
		fmt.Println("  Possible causes:")
		fmt.Println("  - Not running on Azure")
		fmt.Println("  - Network connectivity issues")
		fmt.Println("  - IMDS endpoint blocked by firewall")
		return nil, fmt.Errorf("IMDS not available: %w", result.IMDSError)
	}

	fmt.Println("[PASSED] IMDS Check")
	fmt.Printf("  VM: %s\n", result.VMMetadata.VMName)
	fmt.Printf("  Resource Group: %s\n", result.VMMetadata.ResourceGroup)
	fmt.Printf("  Subscription: %s\n", result.VMMetadata.SubscriptionId)

	// Display and validate managed identity
	if !result.TokenSuccess {
		fmt.Println("")
		fmt.Println("[FAILED] Managed Identity Check")
		fmt.Printf("  Error: %v\n", result.TokenError)
		fmt.Println("  The VM's Managed Identity is not properly configured.")
		fmt.Println("  Required actions:")
		fmt.Println("  1. Enable System-Assigned Managed Identity on this VM")
		fmt.Println("  2. Grant the identity 'Virtual Machine Contributor' role")
		fmt.Println("  3. Ensure the role is scoped to this VM or resource group")
		return nil, fmt.Errorf("managed identity not configured: %w", result.TokenError)
	}

	fmt.Println("[PASSED] Managed Identity Check")
	fmt.Println("  Successfully retrieved access token from IMDS")

	// Display and validate hibernation API access
	if result.HibernationAPIError != nil {
		fmt.Println("")
		fmt.Println("[FAILED] VM Hibernation API Check")
		fmt.Printf("  Error: %v\n", result.HibernationAPIError)
		fmt.Println("  Could not verify VM hibernation capability via Azure API.")
		fmt.Println("  Possible causes:")
		fmt.Println("  - Managed Identity lacks 'Virtual Machine Contributor' or 'Reader' role")
		fmt.Println("  - Role assignment not yet propagated (can take a few minutes)")
		fmt.Println("  - Network connectivity issues to Azure Management API")
		return nil, fmt.Errorf("hibernation API check failed: %w", result.HibernationAPIError)
	}

	// Display hibernation feature status (warning only, not fatal)
	if !result.HibernationEnabled {
		fmt.Println("")
		fmt.Println("[DISABLED] VM Hibernation Feature")
		fmt.Println("  Hibernation is not enabled on this VM.")
		fmt.Println("  Required actions:")
		fmt.Println("  1. Stop (deallocate) this VM in Azure Portal")
		fmt.Println("  2. Enable hibernation in VM settings")
		fmt.Println("  3. Ensure VM size supports hibernation (e.g., D-series, E-series)")
		fmt.Println("  4. Ensure OS disk is configured for hibernation")
		fmt.Println("  5. Restart the VM")
		fmt.Println("")
		fmt.Println("  Note: The service will be installed but hibernation will fail until this is fixed.")
		fmt.Println("  You may proceed with installation if you plan to enable hibernation later.")
	} else {
		fmt.Println("[ENABLED] VM Hibernation Feature")
		fmt.Println("  VM is configured for hibernation in Azure")
	}

	fmt.Println("")
	fmt.Println("All prerequisite checks passed!")
	fmt.Println("This VM can communicate with Azure and authenticate successfully.")

	return result, nil
}

// Install orchestrates the service installation process.
// It tests Azure capabilities, registers the event log source, and creates the Windows service.
func Install() error {
	// Check if running as administrator
	admin, err := isAdmin()
	if err != nil {
		return fmt.Errorf("failed to check administrator privileges: %w", err)
	}
	if !admin {
		fmt.Println("")
		fmt.Println("ERROR: Administrator privileges required")
		fmt.Println("Please run this command in an elevated Command Prompt or PowerShell:")
		fmt.Println("  1. Right-click Command Prompt or PowerShell")
		fmt.Println("  2. Select 'Run as administrator'")
		fmt.Println("  3. Run the install command again")
		return fmt.Errorf("administrator privileges required for service installation")
	}

	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	fmt.Println("")
	fmt.Println("=== Installing Service ===")
	fmt.Println("")

	// Check for notifier executable
	exeDir := filepath.Dir(exePath)
	notifierPath := filepath.Join(exeDir, appinfo.NotifierExeName)
	if _, err := os.Stat(notifierPath); err != nil {
		fmt.Println("[WARNING] Notifier executable not found")
		fmt.Printf("  Expected location: %s\n", notifierPath)
		fmt.Println("  User notifications will not be sent.")
		fmt.Printf("  Please ensure both %s and %s are in the same directory.\n", appinfo.MainExeName, appinfo.NotifierExeName)
		fmt.Println("")
	} else {
		fmt.Println("[OK] Notifier executable found")
		fmt.Printf("  Location: %s\n", notifierPath)
		fmt.Println("")
	}

	// Test Azure hibernation capabilities
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if _, err := testAzureCapabilities(ctx); err != nil {
		return err
	}

	// Register the event log source
	fmt.Println("Registering event log source...")
	if err := registerEventLogSource(appinfo.ServiceName); err != nil {
		return err
	}

	// Connect to the service manager and create the service
	fmt.Println("")
	fmt.Println("Creating Windows service...")
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	// Check if service already exists
	s, err := m.OpenService(appinfo.ServiceName)
	if err == nil {
		s.Close()
		return fmt.Errorf("service %s already exists. Please uninstall it first using -uninstall flag", appinfo.ServiceName)
	}

	// Create the service
	s, err = m.CreateService(
		appinfo.ServiceName,
		exePath,
		mgr.Config{
			DisplayName: appinfo.Name,
			Description: "Monitors VM idleness and hibernates when idle",
			StartType:   mgr.StartAutomatic,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}
	defer s.Close()

	fmt.Println("")
	fmt.Printf("[SUCCESS] Service '%s' created successfully\n", appinfo.ServiceName)
	fmt.Println("  - Start type: Automatic")
	fmt.Println("  - Description: Monitors VM idleness and hibernates when idle")
	fmt.Println("")

	// Start the service
	fmt.Printf("Starting service '%s'...\n", appinfo.ServiceName)
	err = s.Start()
	if err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}
	fmt.Println("  [OK] Service started successfully")
	fmt.Println("")
	fmt.Println("The service is now running and monitoring VM idleness.")
	fmt.Println("You can manage it through Windows Services (services.msc)")

	return nil
}

// Uninstall stops and removes the Windows service.
func Uninstall() error {
	// Check if running as administrator
	admin, err := isAdmin()
	if err != nil {
		return fmt.Errorf("failed to check administrator privileges: %w", err)
	}
	if !admin {
		fmt.Println("")
		fmt.Println("ERROR: Administrator privileges required")
		fmt.Println("Please run this command in an elevated Command Prompt or PowerShell:")
		fmt.Println("  1. Right-click Command Prompt or PowerShell")
		fmt.Println("  2. Select 'Run as administrator'")
		fmt.Println("  3. Run the uninstall command again")
		return fmt.Errorf("administrator privileges required for service uninstallation")
	}

	fmt.Println("")
	fmt.Println("=== Uninstalling Service ===")
	fmt.Println("")

	// Connect to the service manager
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	// Open the service
	s, err := m.OpenService(appinfo.ServiceName)
	if err != nil {
		return fmt.Errorf("service %s not found. It may already be uninstalled", appinfo.ServiceName)
	}
	defer s.Close()

	// Check service status
	status, err := s.Query()
	if err != nil {
		return fmt.Errorf("failed to query service status: %w", err)
	}

	// Stop the service if it's running
	if status.State != svc.Stopped {
		fmt.Printf("Stopping service '%s'...\n", appinfo.ServiceName)
		status, err = s.Control(svc.Stop)
		if err != nil {
			return fmt.Errorf("failed to stop service: %w", err)
		}

		// Wait for service to stop (with timeout)
		timeout := time.Now().Add(30 * time.Second)
		for status.State != svc.Stopped {
			if time.Now().After(timeout) {
				return fmt.Errorf("timeout waiting for service to stop")
			}
			time.Sleep(300 * time.Millisecond)
			status, err = s.Query()
			if err != nil {
				return fmt.Errorf("failed to query service status: %w", err)
			}
		}
		fmt.Println("  [OK] Service stopped successfully")
		fmt.Println("")
	}

	// Delete the service
	fmt.Printf("Deleting service '%s'...\n", appinfo.ServiceName)
	err = s.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete service: %w", err)
	}
	fmt.Println("  [OK] Service deleted successfully")
	fmt.Println("")

	// Remove the event log source
	fmt.Println("Removing event log source...")
	err = eventlog.Remove(appinfo.ServiceName)
	if err != nil {
		// Don't fail if event log removal fails, just warn
		fmt.Printf("  [WARNING] Failed to remove event log source: %v\n", err)
	} else {
		fmt.Println("  [OK] Event log source removed successfully")
	}

	fmt.Println("")
	fmt.Printf("[SUCCESS] Service '%s' has been successfully uninstalled\n", appinfo.ServiceName)

	return nil
}
