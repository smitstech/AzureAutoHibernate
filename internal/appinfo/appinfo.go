//go:build windows

package appinfo

// Application identity constants
const (
	// Name is the display name of the application
	Name = "Azure Auto Hibernate"

	// ServiceName is the Windows service name (no spaces for command-line compatibility)
	ServiceName = "AzureAutoHibernate"

	// MainExeName is the name of the main service executable
	MainExeName = "AzureAutoHibernate.exe"

	// NotifierExeName is the name of the notifier executable
	NotifierExeName = "AzureAutoHibernate.Notifier.exe"

	// UpdaterExeName is the name of the updater helper executable
	UpdaterExeName = "AzureAutoHibernate.Updater.exe"

	// IconFileName is the name of the application icon file
	IconFileName = "AzureAutoHibernate.png"

	// GitHub repository for updates
	RepoOwner = "smitstech"
	RepoName  = "AzureAutoHibernate"
)
