//go:build windows

package pipe

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/smitstech/AzureAutoHibernate/internal/logger"
	"golang.org/x/sys/windows"
)

// Server represents a named pipe server that sends commands to notifiers
type Server struct {
	pipeName string
	logger   logger.Logger
	mu       sync.Mutex
}

// NewServer creates a new pipe server
func NewServer(sessionID int, logger logger.Logger) *Server {
	return &Server{
		pipeName: PipeName(sessionID),
		logger:   logger,
	}
}

// SendCommand sends a command to the notifier and waits for a response
func (s *Server) SendCommand(cmd NotifyCommand) (*NotifyResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Set timestamp if not already set
	if cmd.Timestamp.IsZero() {
		cmd.Timestamp = time.Now()
	}

	s.logger.Debugf(1, "Sending command to notifier: type=%s", cmd.Type)

	// Open named pipe (client mode - connecting to the notifier's pipe server)
	path, err := windows.UTF16PtrFromString(s.pipeName)
	if err != nil {
		return nil, fmt.Errorf("invalid pipe name: %w", err)
	}

	// Try to open the pipe with a timeout
	const timeout = 5 * time.Second
	deadline := time.Now().Add(timeout)

	var handle windows.Handle
	for {
		handle, err = windows.CreateFile(
			path,
			windows.GENERIC_READ|windows.GENERIC_WRITE,
			0,
			nil,
			windows.OPEN_EXISTING,
			windows.FILE_ATTRIBUTE_NORMAL,
			0,
		)

		if err == nil {
			break
		}

		// Check if it's a "file not found" error (pipe doesn't exist)
		if err == windows.ERROR_FILE_NOT_FOUND {
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("notifier not available (pipe not found)")
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Check if pipe is busy
		if err == windows.ERROR_PIPE_BUSY {
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("notifier busy (timeout waiting for pipe)")
			}
			// Wait a bit and retry
			time.Sleep(100 * time.Millisecond)
			continue
		}

		return nil, fmt.Errorf("failed to open pipe: %w", err)
	}
	defer windows.CloseHandle(handle)

	// Send command
	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal command: %w", err)
	}

	// Add newline delimiter
	cmdBytes = append(cmdBytes, '\n')

	var written uint32
	err = windows.WriteFile(handle, cmdBytes, &written, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to write to pipe: %w", err)
	}

	s.logger.Debugf(1, "Sent %d bytes to notifier", written)

	// Read response
	buf := make([]byte, 4096)
	var read uint32
	err = windows.ReadFile(handle, buf, &read, nil)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("failed to read from pipe: %w", err)
	}

	if read == 0 {
		return nil, fmt.Errorf("no response from notifier")
	}

	s.logger.Debugf(1, "Received %d bytes from notifier", read)

	// Parse response
	var response NotifyResponse
	err = json.Unmarshal(buf[:read], &response)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if response.Status == ResponseError {
		return &response, fmt.Errorf("notifier error: %s", response.Error)
	}

	return &response, nil
}

// SendCommandNoWait sends a command without waiting for a response
func (s *Server) SendCommandNoWait(cmd NotifyCommand) error {
	// We'll still use SendCommand but ignore the response
	// This keeps the implementation simpler
	_, err := s.SendCommand(cmd)
	return err
}

// Ping checks if the notifier is available
func (s *Server) Ping() error {
	cmd := NotifyCommand{
		Type:      CommandPing,
		Timestamp: time.Now(),
	}
	_, err := s.SendCommand(cmd)
	return err
}
