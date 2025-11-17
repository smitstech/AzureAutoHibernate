//go:build windows

package notifier

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

const (
	logFileName = "azureautohibernate-notifier.log"
)

// FileLogger logs to a file in the user's temp directory
type FileLogger struct {
	logger *log.Logger
	file   *os.File
}

// NewFileLogger creates a new file logger
func NewFileLogger() (*FileLogger, error) {
	// Create log file in user's temp directory
	tempDir := os.TempDir()
	logPath := filepath.Join(tempDir, logFileName)

	// Open log file (append mode)
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	logger := log.New(file, "", log.LstdFlags)

	return &FileLogger{
		logger: logger,
		file:   file,
	}, nil
}

// Debug logs a debug message
func (l *FileLogger) Debug(msg string) {
	l.logger.Printf("[DEBUG] %s", msg)
}

// Info logs an info message
func (l *FileLogger) Info(msg string) {
	l.logger.Printf("[INFO] %s", msg)
}

// Error logs an error message
func (l *FileLogger) Error(msg string) {
	l.logger.Printf("[ERROR] %s", msg)
}

// Close closes the log file
func (l *FileLogger) Close() error {
	return l.file.Close()
}
