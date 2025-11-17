//go:build windows

package monitor

import (
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	WTS_CURRENT_SERVER_HANDLE = 0
)

// Session state constants
const (
	WTSActive       = 0
	WTSConnected    = 1
	WTSConnectQuery = 2
	WTSShadow       = 3
	WTSDisconnected = 4
	WTSIdle         = 5
	WTSListen       = 6
	WTSReset        = 7
	WTSDown         = 8
	WTSInit         = 9
)

type SessionInfo struct {
	SessionId      uint32
	Username       string
	State          uint32
	IsActive       bool
	IsDisconnected bool
}

var (
	wtsapi32                 = windows.NewLazySystemDLL("wtsapi32.dll")
	procWTSEnumerateSessions = wtsapi32.NewProc("WTSEnumerateSessionsW")
	procWTSQuerySessionInfo  = wtsapi32.NewProc("WTSQuerySessionInformationW")
	procWTSFreeMemory        = wtsapi32.NewProc("WTSFreeMemory")

	kernel32           = windows.NewLazySystemDLL("kernel32.dll")
	procGetTickCount64 = kernel32.NewProc("GetTickCount64")
)

type WTS_SESSION_INFO struct {
	SessionId       uint32
	pWinStationName *uint16
	State           uint32
}

const (
	WTSUserName    = 5
	WTSSessionInfo = 24
)

// WTSINFO structure - only including fields we need
type WTSINFO struct {
	State                   uint32
	SessionId               uint32
	IncomingBytes           uint32
	OutgoingBytes           uint32
	IncomingFrames          uint32
	OutgoingFrames          uint32
	IncomingCompressedBytes uint32
	OutgoingCompressedBytes uint32
	WinStationName          [32]uint16
	Domain                  [17]uint16
	UserName                [21]uint16
	ConnectTime             int64 // LARGE_INTEGER (FILETIME format)
	DisconnectTime          int64
	LastInputTime           int64
	LogonTime               int64
	CurrentTime             int64
}

// GetSessionIdleTime returns the idle time for a specific session
// Returns the duration since last input for that session
func GetSessionIdleTime(sessionId uint32) (time.Duration, error) {
	var buffer *WTSINFO
	var bytesReturned uint32

	ret, _, err := procWTSQuerySessionInfo.Call(
		WTS_CURRENT_SERVER_HANDLE,
		uintptr(sessionId),
		WTSSessionInfo,
		uintptr(unsafe.Pointer(&buffer)),
		uintptr(unsafe.Pointer(&bytesReturned)),
	)

	if ret == 0 {
		return 0, fmt.Errorf("WTSQuerySessionInformation failed for session %d: %v", sessionId, err)
	}
	defer procWTSFreeMemory.Call(uintptr(unsafe.Pointer(buffer)))

	// CurrentTime and LastInputTime are in FILETIME format (100-nanosecond intervals since 1601)
	// Calculate idle time
	idleTime := time.Duration(buffer.CurrentTime-buffer.LastInputTime) * 100 * time.Nanosecond

	return idleTime, nil
}

// GetActiveSessions returns information about all user sessions (Active or Disconnected state only)
// Filters out system sessions, listener sessions, and other non-user session types
func GetActiveSessions() ([]SessionInfo, error) {
	var pSessionInfo *WTS_SESSION_INFO
	var count uint32

	ret, _, err := procWTSEnumerateSessions.Call(
		WTS_CURRENT_SERVER_HANDLE,
		0,
		1,
		uintptr(unsafe.Pointer(&pSessionInfo)),
		uintptr(unsafe.Pointer(&count)),
	)

	if ret == 0 {
		return nil, fmt.Errorf("WTSEnumerateSessions failed: %v", err)
	}
	defer procWTSFreeMemory.Call(uintptr(unsafe.Pointer(pSessionInfo)))

	sessions := make([]SessionInfo, 0)
	size := unsafe.Sizeof(WTS_SESSION_INFO{})

	for i := uint32(0); i < count; i++ {
		session := (*WTS_SESSION_INFO)(unsafe.Pointer(uintptr(unsafe.Pointer(pSessionInfo)) + uintptr(i)*size))

		// Skip system sessions (session 0 is usually services)
		if session.SessionId == 0 {
			continue
		}

		// Only include actual user sessions (Active or Disconnected)
		// Skip listener, shadow, idle, and other non-user session states
		if session.State != WTSActive && session.State != WTSDisconnected {
			continue
		}

		username, err := getSessionUsername(session.SessionId)
		if err != nil || username == "" {
			continue
		}

		info := SessionInfo{
			SessionId:      session.SessionId,
			Username:       username,
			State:          session.State,
			IsActive:       session.State == WTSActive,
			IsDisconnected: session.State == WTSDisconnected,
		}

		sessions = append(sessions, info)
	}

	return sessions, nil
}

func getSessionUsername(sessionId uint32) (string, error) {
	var buffer *uint16
	var bytesReturned uint32

	ret, _, err := procWTSQuerySessionInfo.Call(
		WTS_CURRENT_SERVER_HANDLE,
		uintptr(sessionId),
		WTSUserName,
		uintptr(unsafe.Pointer(&buffer)),
		uintptr(unsafe.Pointer(&bytesReturned)),
	)

	if ret == 0 {
		return "", fmt.Errorf("WTSQuerySessionInformation failed: %v", err)
	}
	defer procWTSFreeMemory.Call(uintptr(unsafe.Pointer(buffer)))

	return windows.UTF16PtrToString(buffer), nil
}

// GetSystemUptime returns the duration since the system was last booted
// Uses GetTickCount64 which returns milliseconds since boot
func GetSystemUptime() (time.Duration, error) {
	ret, _, err := procGetTickCount64.Call()
	if ret == 0 {
		return 0, fmt.Errorf("GetTickCount64 failed: %v", err)
	}

	// GetTickCount64 returns milliseconds since boot
	uptimeMs := ret
	return time.Duration(uptimeMs) * time.Millisecond, nil
}
