//go:build windows

package notifier

import (
	"fmt"
	"os"
	"time"

	"github.com/smitstech/AzureAutoHibernate/assets"
	"github.com/smitstech/AzureAutoHibernate/internal/appinfo"
	"github.com/smitstech/AzureAutoHibernate/internal/pipe"
)

const (
	warningNotificationTag = "hibernation-warning"
	iconTempFilePattern    = "azureautohibernate-icon-*.png"
)

// UI handles displaying notifications to the user
type UI struct {
	logger Logger
}

// NewUI creates a new UI handler
func NewUI(logger Logger) *UI {
	return &UI{
		logger: logger,
	}
}

// HandleCommand processes a command and displays the appropriate UI
func (u *UI) HandleCommand(cmd pipe.NotifyCommand) pipe.NotifyResponse {
	response := pipe.NotifyResponse{
		Status:    pipe.ResponseDisplayed,
		Timestamp: time.Now(),
	}

	switch cmd.Type {
	case pipe.CommandWarning:
		err := u.showWarning(cmd.Message)
		if err != nil {
			u.logger.Error(fmt.Sprintf("Failed to show warning: %v", err))
			response.Status = pipe.ResponseError
			response.Error = err.Error()
		}

	case pipe.CommandCancel:
		err := u.showCancellation(cmd.Message)
		if err != nil {
			u.logger.Error(fmt.Sprintf("Failed to show cancellation: %v", err))
			response.Status = pipe.ResponseError
			response.Error = err.Error()
		}

	case pipe.CommandDismiss:
		// Currently a no-op as go-toast doesn't support programmatic dismissal
		u.logger.Debug("Dismissal command received (no-op)")

	case pipe.CommandPing:
		response.Status = pipe.ResponsePong
		u.logger.Debug("Ping received")

	case pipe.CommandInfo:
		err := u.showInfo(cmd.Message)
		if err != nil {
			u.logger.Error(fmt.Sprintf("Failed to show info notification: %v", err))
			response.Status = pipe.ResponseError
			response.Error = err.Error()
		}

	default:
		u.logger.Error(fmt.Sprintf("Unknown command type: %s", cmd.Type))
		response.Status = pipe.ResponseError
		response.Error = fmt.Sprintf("unknown command type: %s", cmd.Type)
	}

	return response
}

// showWarning displays a hibernation warning notification
func (u *UI) showWarning(message string) error {
	if message == "" {
		return fmt.Errorf("warning message is required")
	}

	title := "VM Hibernation Warning"
	return u.sendToastNotification(title, message, warningNotificationTag)
}

// showCancellation displays a hibernation cancellation notification
func (u *UI) showCancellation(message string) error {
	if message == "" {
		return fmt.Errorf("cancellation message is required")
	}

	title := "Hibernation Canceled"
	return u.sendToastNotification(title, message, "")
}

// showInfo displays an informational notification
func (u *UI) showInfo(message string) error {
	if message == "" {
		return fmt.Errorf("info message is required")
	}

	title := appinfo.Name
	return u.sendToastNotification(title, message, "")
}

// sendToastNotification creates a Windows 10/11 toast notification
func (u *UI) sendToastNotification(title, message, tag string) error {
	// Get icon path (writes embedded icon to temp file)
	iconPath, err := getIconPath()
	if err != nil {
		// Log error but don't fail the notification
		u.logger.Error(fmt.Sprintf("Failed to get icon path: %v", err))
		iconPath = ""
	} else if iconPath != "" {
		// Clean up temp file after notification is sent
		defer os.Remove(iconPath)
	}

	// Create notification with appropriate audio settings
	var notification ToastNotification
	if tag == warningNotificationTag {
		notification = ToastNotification{
			AppID:    appinfo.Name,
			Title:    title,
			Message:  message,
			IconPath: iconPath,
			Audio:    AudioReminder,
			Duration: DurationLong,
			Tag:      tag,
		}
	} else {
		notification = ToastNotification{
			AppID:    appinfo.Name,
			Title:    title,
			Message:  message,
			IconPath: iconPath,
			Audio:    AudioSilent,
			Duration: DurationShort,
		}
	}

	err = notification.Show()
	if err != nil {
		return fmt.Errorf("failed to send toast notification: %w", err)
	}

	u.logger.Info(fmt.Sprintf("Toast notification sent: %s", title))
	return nil
}

// getIconPath writes the embedded icon to a temporary file and returns the path
func getIconPath() (string, error) {
	// Create temp file with .png extension
	tmpFile, err := os.CreateTemp("", iconTempFilePattern)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tmpFile.Close()

	// Write embedded icon data
	if _, err := tmpFile.Write(assets.IconData); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to write icon data: %w", err)
	}

	return tmpFile.Name(), nil
}
