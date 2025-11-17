//go:build windows

package notifier

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/smitstech/AzureAutoHibernate/internal/pipe"
	"golang.org/x/sys/windows"
)

// PipeClient represents the notifier's pipe server that receives commands
type PipeClient struct {
	pipeName  string
	sessionID int
	handler   CommandHandler
	stopChan  chan struct{}
	wg        sync.WaitGroup
	logger    Logger
}

// CommandHandler handles commands received from the service
type CommandHandler interface {
	HandleCommand(cmd pipe.NotifyCommand) pipe.NotifyResponse
}

// Logger interface for logging
type Logger interface {
	Debug(msg string)
	Info(msg string)
	Error(msg string)
}

// NewPipeClient creates a new pipe client
func NewPipeClient(sessionID int, handler CommandHandler, logger Logger) *PipeClient {
	return &PipeClient{
		pipeName:  pipe.PipeName(sessionID),
		sessionID: sessionID,
		handler:   handler,
		stopChan:  make(chan struct{}),
		logger:    logger,
	}
}

// Start begins listening for commands on the named pipe
func (c *PipeClient) Start() error {
	c.logger.Info(fmt.Sprintf("Starting pipe client on %s", c.pipeName))

	c.wg.Add(1)
	go c.listenLoop()

	return nil
}

// Stop stops the pipe client
func (c *PipeClient) Stop() {
	c.logger.Info("Stopping pipe client")
	close(c.stopChan)
	c.wg.Wait()
}

func (c *PipeClient) listenLoop() {
	defer c.wg.Done()

	for {
		select {
		case <-c.stopChan:
			c.logger.Info("Pipe client stopped")
			return
		default:
			err := c.acceptConnection()
			if err != nil {
				select {
				case <-c.stopChan:
					return
				default:
					c.logger.Error(fmt.Sprintf("Error accepting connection: %v", err))
				}
			}
		}
	}
}

func (c *PipeClient) acceptConnection() error {
	// Create named pipe server
	path, err := windows.UTF16PtrFromString(c.pipeName)
	if err != nil {
		return fmt.Errorf("invalid pipe name: %w", err)
	}

	// Create named pipe with default security (allows same user and SYSTEM)
	handle, err := windows.CreateNamedPipe(
		path,
		windows.PIPE_ACCESS_DUPLEX,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT,
		windows.PIPE_UNLIMITED_INSTANCES,
		4096, // output buffer size
		4096, // input buffer size
		0,    // default timeout
		nil,  // default security attributes
	)

	if err != nil {
		return fmt.Errorf("failed to create named pipe: %w", err)
	}
	defer windows.CloseHandle(handle)

	c.logger.Debug("Waiting for connection...")

	// Wait for a client to connect
	err = windows.ConnectNamedPipe(handle, nil)
	if err != nil && err != windows.ERROR_PIPE_CONNECTED {
		return fmt.Errorf("failed to connect named pipe: %w", err)
	}

	c.logger.Debug("Client connected")

	// Handle the connection
	err = c.handleConnection(handle)
	if err != nil {
		c.logger.Error(fmt.Sprintf("Error handling connection: %v", err))
	}

	// Pipe will be closed by defer above

	return nil
}

func (c *PipeClient) handleConnection(handle windows.Handle) error {
	// Read command
	buf := make([]byte, 4096)
	var read uint32
	err := windows.ReadFile(handle, buf, &read, nil)
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to read from pipe: %w", err)
	}

	if read == 0 {
		return fmt.Errorf("no data received")
	}

	c.logger.Debug(fmt.Sprintf("Received %d bytes", read))

	// Parse command - unmarshal the trimmed buffer
	data := buf[:read]
	// Remove trailing newline if present
	if len(data) > 0 && data[len(data)-1] == '\n' {
		data = data[:len(data)-1]
	}

	var cmd pipe.NotifyCommand
	err = json.Unmarshal(data, &cmd)
	if err != nil {
		return fmt.Errorf("failed to unmarshal command: %w", err)
	}

	c.logger.Debug(fmt.Sprintf("Received command: type=%s", cmd.Type))

	// Handle command
	response := c.handler.HandleCommand(cmd)
	response.SessionID = c.sessionID

	// Send response
	responseBytes, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	var written uint32
	err = windows.WriteFile(handle, responseBytes, &written, nil)
	if err != nil {
		return fmt.Errorf("failed to write response: %w", err)
	}

	c.logger.Debug(fmt.Sprintf("Sent %d bytes response", written))

	return nil
}
