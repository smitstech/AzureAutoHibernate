//go:build windows

package pipe

import (
	"fmt"
	"time"
)

// CommandType represents the type of command sent from service to notifier
type CommandType string

const (
	CommandWarning CommandType = "warning"
	CommandCancel  CommandType = "cancel"
	CommandDismiss CommandType = "dismiss"
	CommandPing    CommandType = "ping"
	CommandInfo    CommandType = "info"

	// pipeNamePrefix is the prefix for named pipe names
	pipeNamePrefix = `\\.\pipe\azureautohibernate-notify`
)

// ResponseStatus represents the status of a notifier response
type ResponseStatus string

const (
	ResponseDisplayed  ResponseStatus = "displayed"
	ResponseUserCancel ResponseStatus = "user_cancel"
	ResponseError      ResponseStatus = "error"
	ResponsePong       ResponseStatus = "pong"
)

// NotifyCommand is sent from the service to the notifier
type NotifyCommand struct {
	Type          CommandType `json:"type"`
	TimeRemaining int         `json:"timeRemaining,omitempty"` // seconds
	Reason        string      `json:"reason,omitempty"`
	Message       string      `json:"message,omitempty"`
	Timestamp     time.Time   `json:"timestamp"`
}

// NotifyResponse is sent from the notifier to the service
type NotifyResponse struct {
	Status    ResponseStatus `json:"status"`
	SessionID int            `json:"sessionId"`
	Error     string         `json:"error,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

// PipeName returns the named pipe path for a given session
func PipeName(sessionID int) string {
	// Each session gets its own named pipe to avoid conflicts
	return fmt.Sprintf(`%s-%d`, pipeNamePrefix, sessionID)
}
