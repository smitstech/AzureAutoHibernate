//go:build windows

package logger

import (
	"fmt"
	"log"
	"strings"

	"golang.org/x/sys/windows/svc/eventlog"
)

// LogLevel represents the logging level
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarning
	LevelError
)

// ParseLogLevel converts a string to a LogLevel
func ParseLogLevel(level string) LogLevel {
	switch strings.ToLower(level) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarning
	case "error":
		return LevelError
	default:
		return LevelInfo // default to info
	}
}

// String returns the string representation of a LogLevel
func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarning:
		return "WARNING"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Event IDs for different types of events
const (
	// Service lifecycle events (1-9)
	EventServiceStart      = 1
	EventServiceStop       = 2
	EventConfigLoaded      = 3
	EventMonitoringStarted = 4

	// Idle monitoring - informational events (5-9)
	EventIdleCheckInfo            = 5
	EventSessionSummary           = 6
	EventUserActivity             = 7
	EventIdleThresholdMet         = 8
	EventIdleConditionNoLongerMet = 9

	// Hibernation warning events (10-19)
	EventHibernationWarningStart  = 10
	EventHibernationWarningSent   = 11
	EventHibernationWarningCancel = 12
	EventHibernationTriggered     = 13
	EventHibernationSuccess       = 14
	EventWarningPeriodActive      = 15
	EventWarningReasonChanged     = 16

	// Warning events (20-29)
	EventSessionInfoWarning  = 20
	EventIdleCheckWarning    = 21
	EventNotificationWarning = 22

	// Error events (30-39)
	EventConfigError         = 30
	EventSessionMonitorError = 31
	EventIdleCheckError      = 32
	EventHibernationError    = 33
	EventAzureAuthError      = 34
	EventNotificationError   = 35
)

// Logger provides a unified interface for logging to Windows Event Log or console
type Logger interface {
	Debug(eventID uint32, msg string)
	Info(eventID uint32, msg string)
	Warning(eventID uint32, msg string)
	Error(eventID uint32, msg string)
	Debugf(eventID uint32, format string, args ...interface{})
	Infof(eventID uint32, format string, args ...interface{})
	Warningf(eventID uint32, format string, args ...interface{})
	Errorf(eventID uint32, format string, args ...interface{})
	Close() error
}

// EventLogger writes to Windows Event Log
type EventLogger struct {
	elog  *eventlog.Log
	level LogLevel
}

// ConsoleLogger writes to console (for debug mode)
type ConsoleLogger struct {
	level LogLevel
}

// NewEventLogger creates a logger that writes to Windows Event Log
func NewEventLogger(serviceName string, level LogLevel) (*EventLogger, error) {
	elog, err := eventlog.Open(serviceName)
	if err != nil {
		return nil, fmt.Errorf("failed to open event log: %w", err)
	}
	return &EventLogger{elog: elog, level: level}, nil
}

// NewConsoleLogger creates a logger that writes to console
func NewConsoleLogger(level LogLevel) *ConsoleLogger {
	return &ConsoleLogger{level: level}
}

// EventLogger methods
func (l *EventLogger) Debug(eventID uint32, msg string) {
	if l.level <= LevelDebug {
		l.elog.Info(eventID, "[DEBUG] "+msg)
	}
}

func (l *EventLogger) Info(eventID uint32, msg string) {
	if l.level <= LevelInfo {
		l.elog.Info(eventID, msg)
	}
}

func (l *EventLogger) Warning(eventID uint32, msg string) {
	if l.level <= LevelWarning {
		l.elog.Warning(eventID, msg)
	}
}

func (l *EventLogger) Error(eventID uint32, msg string) {
	if l.level <= LevelError {
		l.elog.Error(eventID, msg)
	}
}

func (l *EventLogger) Debugf(eventID uint32, format string, args ...interface{}) {
	if l.level <= LevelDebug {
		l.elog.Info(eventID, "[DEBUG] "+fmt.Sprintf(format, args...))
	}
}

func (l *EventLogger) Infof(eventID uint32, format string, args ...interface{}) {
	if l.level <= LevelInfo {
		l.elog.Info(eventID, fmt.Sprintf(format, args...))
	}
}

func (l *EventLogger) Warningf(eventID uint32, format string, args ...interface{}) {
	if l.level <= LevelWarning {
		l.elog.Warning(eventID, fmt.Sprintf(format, args...))
	}
}

func (l *EventLogger) Errorf(eventID uint32, format string, args ...interface{}) {
	if l.level <= LevelError {
		l.elog.Error(eventID, fmt.Sprintf(format, args...))
	}
}

func (l *EventLogger) Close() error {
	return l.elog.Close()
}

// ConsoleLogger methods (event IDs are ignored in console mode)
func (l *ConsoleLogger) Debug(eventID uint32, msg string) {
	if l.level <= LevelDebug {
		log.Printf("[DEBUG] [%d] %s", eventID, msg)
	}
}

func (l *ConsoleLogger) Info(eventID uint32, msg string) {
	if l.level <= LevelInfo {
		log.Printf("[INFO] [%d] %s", eventID, msg)
	}
}

func (l *ConsoleLogger) Warning(eventID uint32, msg string) {
	if l.level <= LevelWarning {
		log.Printf("[WARN] [%d] %s", eventID, msg)
	}
}

func (l *ConsoleLogger) Error(eventID uint32, msg string) {
	if l.level <= LevelError {
		log.Printf("[ERROR] [%d] %s", eventID, msg)
	}
}

func (l *ConsoleLogger) Debugf(eventID uint32, format string, args ...interface{}) {
	if l.level <= LevelDebug {
		log.Printf("[DEBUG] [%d] "+format, append([]interface{}{eventID}, args...)...)
	}
}

func (l *ConsoleLogger) Infof(eventID uint32, format string, args ...interface{}) {
	if l.level <= LevelInfo {
		log.Printf("[INFO] [%d] "+format, append([]interface{}{eventID}, args...)...)
	}
}

func (l *ConsoleLogger) Warningf(eventID uint32, format string, args ...interface{}) {
	if l.level <= LevelWarning {
		log.Printf("[WARN] [%d] "+format, append([]interface{}{eventID}, args...)...)
	}
}

func (l *ConsoleLogger) Errorf(eventID uint32, format string, args ...interface{}) {
	if l.level <= LevelError {
		log.Printf("[ERROR] [%d] "+format, append([]interface{}{eventID}, args...)...)
	}
}

func (l *ConsoleLogger) Close() error {
	return nil
}
