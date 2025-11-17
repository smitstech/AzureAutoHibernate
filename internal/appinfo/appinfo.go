//go:build windows

package appinfo

// Application identity constants
const (
	// Name is the display name of the application
	Name = "AzureAutoHibernate"

	// ServiceName is the Windows service name
	ServiceName = "AzureAutoHibernate"

	// MainExeName is the name of the main service executable
	MainExeName = "AzureAutoHibernate.exe"

	// NotifierExeName is the name of the notifier executable
	NotifierExeName = "AzureAutoHibernate.Notifier.exe"

	// IconFileName is the name of the application icon file
	IconFileName = "AzureAutoHibernate.png"
)
