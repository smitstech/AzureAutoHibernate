# Testing

The project includes comprehensive unit tests (~2,000 lines) covering critical functionality:

## Test Coverage

| Test File                 | Lines | Coverage                                                 |
| ------------------------- | ----- | -------------------------------------------------------- |
| `config/config_test.go`   | 449   | Configuration validation, file loading, edge cases       |
| `monitor/idle_test.go`    | 602   | FSM state transitions, time calculations, idle detection |
| `pipe/messages_test.go`   | 296   | Time formatting, notification messages, rounding logic   |
| `service/service_test.go` | 644   | Service initialization, dynamic polling, power events    |

## Running Tests

```bash
# Run all tests
go test ./...

# Run tests for specific package
go test ./internal/config/...
go test ./internal/monitor/...    # Requires Windows
go test ./internal/pipe/...        # Requires Windows
go test ./internal/service/...    # Requires Windows

# Run with verbose output
go test -v ./...

# Run with coverage
go test -cover ./...
```

**Note:** Tests for `monitor`, `pipe`, and `service` packages require Windows as they use Windows-specific APIs (WTS, Event Log). They use `//go:build windows` build tags and will be skipped on other platforms.

## What's Tested

### Configuration (`config/config_test.go`)

- Configuration validation (negative values, zero thresholds, invalid log levels)
- JSON file loading and parsing
- Default value handling (empty log level defaults to "info")
- Edge cases (all thresholds zero, very large values)
- 25+ test cases for validation logic

### Idle Detection (`monitor/idle_test.go`)

- **FSM state transitions**: None → Active → Canceled/Hibernate
- **Time calculations**: Idle duration, warning period expiration, threshold crossing
- **Minimum uptime boundary**: Verifies `<=` comparison (prevents flapping)
- **Warning cancellation logic**: User activity detection
- **GetTimeUntilThresholds**: Dynamic polling interval calculation
- 15+ comprehensive test suites

### Notifications (`pipe/messages_test.go`)

- **Time formatting**: 30-second rounding logic (`FormatTimeRemaining()`)
- **Message construction**: Warning and cancellation message generation
- **Edge cases**: Zero duration, boundary rounding, consistency
- **Deterministic behavior**: 100 iterations for consistency testing
- **Rounding boundaries**: Comprehensive edge case testing (14s, 15s, 44s, 45s, etc.)
- 25+ test cases for time formatting with comprehensive edge case coverage

### Service Orchestration (`service/service_test.go`)

- **Service initialization**: Proper component setup
- **Dynamic check intervals**: `calculateNextCheckTime()` logic
- **Power event handling**: Resume tracking (PBT_APMRESUMEAUTOMATIC/SUSPEND)
- **Warning mode transitions**: 5-second vs dynamic polling
- **Event logging**: Verification of log output
- 12+ test suites for orchestration

## Test Features

- **Mock logger implementations** for isolated testing without Event Log dependencies
- **Edge case coverage** including boundary conditions (≤ vs <, rounding edge cases)
- **FSM state machine** thoroughly tested with all transition paths
- **Time calculations** validated with tolerance checks for timing precision
- **Windows API mocking** patterns for session monitoring tests

## Benefits

The comprehensive test suite provides:

- **Confidence in refactoring**: Safe code changes without breaking existing behavior
- **Regression prevention**: Catch bugs before they reach production
- **Documentation**: Tests serve as executable examples of expected behavior
- **Coverage of critical logic**: FSM transitions, time calculations, session monitoring
