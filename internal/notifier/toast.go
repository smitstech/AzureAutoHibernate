//go:build windows

package notifier

import (
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// ToastNotification represents a Windows toast notification
type ToastNotification struct {
	AppID    string
	Title    string
	Message  string
	IconPath string
	Audio    ToastAudio
	Duration ToastDuration
	Tag      string
}

// ToastAudio represents the audio type for a toast
type ToastAudio string

const (
	AudioSilent   ToastAudio = "ms-winsoundevent:Notification.Default"
	AudioReminder ToastAudio = "ms-winsoundevent:Notification.Reminder"
)

// ToastDuration represents how long the toast should display
type ToastDuration string

const (
	DurationShort ToastDuration = "short"
	DurationLong  ToastDuration = "long"
)

// toastXML represents the XML structure for Windows toast notifications
type toastXML struct {
	XMLName        xml.Name `xml:"toast"`
	ActivationType string   `xml:"activationType,attr,omitempty"`
	Launch         string   `xml:"launch,attr,omitempty"`
	Duration       string   `xml:"duration,attr,omitempty"`
	Visual         visual   `xml:"visual"`
	Audio          *audio   `xml:"audio,omitempty"`
}

type visual struct {
	Binding binding `xml:"binding"`
}

type binding struct {
	Template string `xml:"template,attr"`
	Image    *image `xml:"image,omitempty"`
	Text     []text `xml:"text"`
}

type image struct {
	ID        string `xml:"id,attr,omitempty"`
	Src       string `xml:"src,attr"`
	Placement string `xml:"placement,attr,omitempty"`
}

type text struct {
	Value string `xml:",cdata"`
}

type audio struct {
	Src    string `xml:"src,attr,omitempty"`
	Silent bool   `xml:"silent,attr,omitempty"`
}

// Show displays the toast notification using PowerShell
func (t *ToastNotification) Show() error {
	// Build the XML for the toast
	toastXMLContent, err := t.buildXML()
	if err != nil {
		return fmt.Errorf("failed to build toast XML: %w", err)
	}

	// Escape XML for PowerShell - single quotes don't need escaping in single-quoted strings
	escapedXML := strings.ReplaceAll(toastXMLContent, "'", "''")

	// Build PowerShell script to show the toast
	script := fmt.Sprintf(`
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null

$APP_ID = '%s'
$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
$xml.LoadXml('%s')
$toast = New-Object Windows.UI.Notifications.ToastNotification $xml
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($APP_ID).Show($toast)
`, t.AppID, escapedXML)

	// Write script to temp file with UTF-8 BOM (like go-toast does)
	tmpFile, err := os.CreateTemp("", "toast-*.ps1")
	if err != nil {
		return fmt.Errorf("failed to create temp script file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write BOM for UTF-8
	bom := []byte{0xEF, 0xBB, 0xBF}
	if _, err := tmpFile.Write(bom); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write BOM: %w", err)
	}

	// Write script content
	if _, err := tmpFile.WriteString(script); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write script: %w", err)
	}
	tmpFile.Close()

	// Execute PowerShell with the script file, hiding the window
	cmd := exec.Command("powershell.exe", "-ExecutionPolicy", "Bypass", "-File", tmpFile.Name())
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to show toast notification: %w (output: %s)", err, string(output))
	}

	return nil
}

// buildXML creates the toast notification XML
func (t *ToastNotification) buildXML() (string, error) {
	toast := toastXML{
		ActivationType: "protocol",
		Duration:       string(t.Duration),
		Visual: visual{
			Binding: binding{
				Template: "ToastGeneric",
				Text: []text{
					{Value: t.Title},
					{Value: t.Message},
				},
			},
		},
	}

	// Add icon if provided
	if t.IconPath != "" {
		// Convert to file:/// URI for local paths
		iconURI := t.IconPath
		if !strings.HasPrefix(iconURI, "file:///") && !strings.HasPrefix(iconURI, "http") {
			iconURI = "file:///" + strings.ReplaceAll(iconURI, "\\", "/")
		}
		toast.Visual.Binding.Image = &image{
			ID:        "1",
			Src:       iconURI,
			Placement: "appLogoOverride",
		}
	}

	// Add audio settings
	if t.Audio == AudioSilent {
		toast.Audio = &audio{Silent: true}
	} else if t.Audio != "" {
		toast.Audio = &audio{Src: string(t.Audio)}
	}

	// Marshal to XML
	xmlData, err := xml.MarshalIndent(toast, "", "  ")
	if err != nil {
		return "", err
	}

	return xml.Header + string(xmlData), nil
}
