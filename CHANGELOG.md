# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/).

---

## [Unreleased]

### Added

- **Version information system** with build-time injection from git tags
  - New `internal/version` package with `Version`, `GitCommit`, and `BuildDate` variables
  - `-version` flag for `AzureAutoHibernate.exe` outputs clean version string (e.g., `v1.0.0`)
- **Build automation scripts**
  - `build.ps1` - PowerShell build script with automatic version detection from git tags
  - `Makefile` - Cross-platform build script for Linux/Mac development
  - Both scripts automatically inject version information via ldflags
  - Build scripts test version output after compilation
- **Release automation improvements**
  - GitHub Actions release workflow now injects version info into both executables
  - Version, commit hash, and build date automatically embedded in release binaries
  - Automatic extraction of release notes from CHANGELOG.md to GitHub release descriptions

### Changed

- Updated build documentation in `README.md` with version checking examples
- Build process now uses git tags as single source of truth for versioning

---

## [1.0.1] – 2025-11-22

### Added

- **Automatic service installation** - the `-install` flag now creates and starts the Windows service automatically
  - New `internal/installer` package handles service registration via Windows Service Control Manager
  - No longer requires manual `sc create` and `sc start` commands
  - Simplified installation: just run `AzureAutoHibernate.exe -install` as Administrator

### Changed

- Refactored installation logic from `cmd/autohibernate/main.go` into dedicated `internal/installer` package

---

## [1.0.0] – 2025-11-17

### Added

#### Core Service (`AzureAutoHibernate.exe`)

- **Windows service** that monitors VM idle state and automatically hibernates via Azure API
- **Three-condition idle detection system** with independent configurable thresholds:
  - `noUsersIdleMinutes` - Hibernate when no users are logged in
  - `allDisconnectedIdleMinutes` - Hibernate when all users are disconnected
  - `inactiveUserIdleMinutes` - Hibernate when logged-in users are inactive (no keyboard/mouse input)
- **Dynamic polling** that adjusts check frequency based on time until next threshold
  - Polls every 5 seconds during warning periods for responsive cancellation
  - Reduces polling frequency when far from thresholds to minimize overhead
- **Minimum uptime enforcement** (`minimumUptimeMinutes`) to prevent boot-hibernate loops
- **Power event handling** that tracks system resume from sleep/hibernate for accurate idle calculations
- **Session monitoring** that distinguishes between:
  - Active console sessions
  - Disconnected RDP sessions
  - Logged in vs. logged out states

#### User Notifications (`AzureAutoHibernate.Notifier.exe`)

- **Session-0 isolation architecture** using named pipes for SYSTEM service to user session communication
- **Per-session notifier processes** automatically launched for each active user session
- **Toast notification warnings** with:
  - Configurable warning period (`inactiveUserWarningMinutes`) for inactive user condition only
  - Real-time countdown showing time remaining until hibernation
  - Detailed reason for pending hibernation
  - Windows 10/11 native toast notifications with custom icon
- **Activity-based cancellation** - any mouse/keyboard input during warning period cancels hibernation
- **Notification throttling** (30-second minimum between repeat warnings)
- **Automatic cancellation notifications** sent when user activity is detected

#### Azure Integration

- **Azure Hibernate API integration** via Azure Instance Metadata Service (IMDS)
- **Automatic VM discovery** - retrieves subscription, resource group, and VM name from IMDS
- **Managed Identity authentication** - no stored credentials required
- Direct link-local endpoint communication (`169.254.169.254`)

#### Configuration (`config.json`)

- **Flexible threshold configuration** with support for:
  - Multiple idle detection strategies (at least one must be > 0)
  - Independent timeouts for different scenarios
  - Warning period (applies only to inactive user condition)
  - Minimum uptime protection
- **Configurable log levels**: `debug`, `info`, `warn`, `error`
- **Graceful degradation** - service continues if notifier is unavailable

#### Logging & Monitoring

- **Event Log integration** with structured, categorized events:
  - Service lifecycle events
  - Idle state detection events
  - Hibernation trigger events
  - Warning and cancellation events
  - Error and diagnostic events
- **Dedicated Event Viewer log** (`Applications and Services Logs → AzureAutoHibernate`)
- **Notifier logging** to user temp directory for debugging

#### Developer Experience

- **Cross-compilation support** with build instructions for Mac/Linux → Windows
- **Centralized application constants** in `internal/appinfo` package
- **Comprehensive test suite** with unit tests for core logic
- **Testing documentation** (`TESTING.md`) with detailed testing scenarios
- **Debug mode** (`-debug` flag) for console-based testing

### Fixed

- **Session idle time validation** to prevent false activity detection
  - Validates that `CurrentTime` is non-zero before calculating idle duration
  - Verifies `LastInputTime <= CurrentTime` to prevent negative durations
  - Returns error for invalid/garbage session data instead of treating as active

### Security

- **No stored Azure credentials** — uses Managed Identity exclusively
- **IMDS calls** use direct link-local endpoint (`169.254.169.254`)
- **Minimal permissions** required (Virtual Machine Contributor scoped to specific VM)
- **Service runs as LocalSystem** to access session information while maintaining security isolation
- **Automatic exit** if not running on Azure to prevent misuse

### Infrastructure / Project

- MIT license
- Project structure:
  - `cmd/autohibernate` - Main service executable
  - `cmd/notifier` - User notification executable
  - `internal/` - Shared packages (monitor, azure, config, logger, notifier, pipe, service, appinfo)
  - `assets/` - Embedded resources (notification icon)
- **GitHub Actions** build automation with Windows test runners
- Documentation:
  - `README.md` - Setup, installation, configuration, and usage
  - `TESTING.md` - Comprehensive testing guide
  - `LICENSE` - MIT license
- **Embedded assets** using Go 1.16+ embed for notification icon
