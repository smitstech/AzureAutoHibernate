package version

import (
	"fmt"
	"runtime"
)

var (
	// Version is the semantic version (set via ldflags)
	Version = "dev"
	// GitCommit is the git commit hash (set via ldflags)
	GitCommit = "unknown"
	// BuildDate is the build date in RFC3339 format (set via ldflags)
	BuildDate = "unknown"
)

// Info returns formatted version information
func Info() string {
	return fmt.Sprintf("Version: %s\nCommit:  %s\nBuilt:   %s\nGo:      %s",
		Version, GitCommit, BuildDate, runtime.Version())
}

// Short returns a short version string
func Short() string {
	return Version
}
