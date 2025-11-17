//go:build windows

package pipe

import (
	"strings"
	"testing"
	"time"
)

// TestFormatTimeRemaining tests the time formatting logic for notifications
func TestFormatTimeRemaining(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{
			name:     "zero duration",
			duration: 0,
			want:     "immediately",
		},
		{
			name:     "negative duration",
			duration: -5 * time.Second,
			want:     "immediately",
		},
		{
			name:     "less than 30 seconds",
			duration: 10 * time.Second,
			want:     "less than 30 seconds",
		},
		{
			name:     "15 seconds - rounds to 30 seconds",
			duration: 15 * time.Second,
			want:     "30 seconds",
		},
		{
			name:     "20 seconds - rounds to 30 seconds",
			duration: 20 * time.Second,
			want:     "30 seconds",
		},
		{
			name:     "29 seconds - rounds to 30 seconds",
			duration: 29 * time.Second,
			want:     "30 seconds",
		},
		{
			name:     "30 seconds exactly",
			duration: 30 * time.Second,
			want:     "30 seconds",
		},
		{
			name:     "44 seconds - rounds to 30 seconds",
			duration: 44 * time.Second,
			want:     "30 seconds",
		},
		{
			name:     "45 seconds - rounds to 1 minute",
			duration: 45 * time.Second,
			want:     "1 minute",
		},
		{
			name:     "59 seconds - rounds to 1 minute",
			duration: 59 * time.Second,
			want:     "1 minute",
		},
		{
			name:     "60 seconds - 1 minute",
			duration: 60 * time.Second,
			want:     "1 minute",
		},
		{
			name:     "74 seconds - rounds to 1 minute",
			duration: 74 * time.Second,
			want:     "1 minute",
		},
		{
			name:     "75 seconds - rounds to 1 minute 30 seconds",
			duration: 75 * time.Second,
			want:     "1 minute 30 seconds",
		},
		{
			name:     "89 seconds - rounds to 1 minute 30 seconds",
			duration: 89 * time.Second,
			want:     "1 minute 30 seconds",
		},
		{
			name:     "90 seconds - 1 minute 30 seconds",
			duration: 90 * time.Second,
			want:     "1 minute 30 seconds",
		},
		{
			name:     "104 seconds - rounds to 1 minute 30 seconds",
			duration: 104 * time.Second,
			want:     "1 minute 30 seconds",
		},
		{
			name:     "105 seconds - rounds to 2 minutes",
			duration: 105 * time.Second,
			want:     "2 minutes",
		},
		{
			name:     "2 minutes",
			duration: 2 * time.Minute,
			want:     "2 minutes",
		},
		{
			name:     "2 minutes 29 seconds - rounds to 2 minutes 30 seconds",
			duration: 2*time.Minute + 29*time.Second,
			want:     "2 minutes 30 seconds",
		},
		{
			name:     "2 minutes 30 seconds",
			duration: 2*time.Minute + 30*time.Second,
			want:     "2 minutes 30 seconds",
		},
		{
			name:     "2 minutes 44 seconds - rounds to 2 minutes 30 seconds",
			duration: 2*time.Minute + 44*time.Second,
			want:     "2 minutes 30 seconds",
		},
		{
			name:     "2 minutes 45 seconds - rounds to 3 minutes",
			duration: 2*time.Minute + 45*time.Second,
			want:     "3 minutes",
		},
		{
			name:     "5 minutes",
			duration: 5 * time.Minute,
			want:     "5 minutes",
		},
		{
			name:     "5 minutes 14 seconds - rounds to 5 minutes",
			duration: 5*time.Minute + 14*time.Second,
			want:     "5 minutes",
		},
		{
			name:     "5 minutes 15 seconds - rounds to 5 minutes 30 seconds",
			duration: 5*time.Minute + 15*time.Second,
			want:     "5 minutes 30 seconds",
		},
		{
			name:     "10 minutes",
			duration: 10 * time.Minute,
			want:     "10 minutes",
		},
		{
			name:     "30 minutes",
			duration: 30 * time.Minute,
			want:     "30 minutes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatTimeRemaining(tt.duration)
			if got != tt.want {
				t.Errorf("FormatTimeRemaining(%v) = %q, want %q", tt.duration, got, tt.want)
			}
		})
	}
}

// TestFormatTimeRemainingRoundingEdgeCases tests edge cases in rounding logic
func TestFormatTimeRemainingRoundingEdgeCases(t *testing.T) {
	// Test the rounding boundaries more thoroughly
	tests := []struct {
		seconds int
		want    string
	}{
		{14, "less than 30 seconds"}, // 14 + 15 = 29, rounds to 0
		{15, "30 seconds"},           // 15 + 15 = 30, rounds to 30
		{44, "30 seconds"},           // 44 + 15 = 59, rounds to 30
		{45, "1 minute"},             // 45 + 15 = 60, rounds to 60
		{74, "1 minute"},             // 74 + 15 = 89, rounds to 60
		{75, "1 minute 30 seconds"},  // 75 + 15 = 90, rounds to 90
		{104, "1 minute 30 seconds"}, // 104 + 15 = 119, rounds to 90
		{105, "2 minutes"},           // 105 + 15 = 120, rounds to 120
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			duration := time.Duration(tt.seconds) * time.Second
			got := FormatTimeRemaining(duration)
			if got != tt.want {
				t.Errorf("FormatTimeRemaining(%ds) = %q, want %q", tt.seconds, got, tt.want)
			}
		})
	}
}

// TestFormatTimeRemainingConsistency verifies consistent behavior across multiple calls
func TestFormatTimeRemainingConsistency(t *testing.T) {
	// Test that the same input always produces the same output
	duration := 5*time.Minute + 15*time.Second
	expected := FormatTimeRemaining(duration)

	for i := 0; i < 100; i++ {
		result := FormatTimeRemaining(duration)
		if result != expected {
			t.Errorf("Iteration %d: FormatTimeRemaining(%v) = %q, want %q (inconsistent!)",
				i, duration, result, expected)
		}
	}
}

// TestFormatWarningMessage tests the warning message formatting
func TestFormatWarningMessage(t *testing.T) {
	tests := []struct {
		name          string
		reason        string
		timeRemaining time.Duration
		wantContains  []string
	}{
		{
			name:          "basic warning",
			reason:        "No user input activity",
			timeRemaining: 5 * time.Minute,
			wantContains:  []string{"5 minutes", "No user input activity", "cancel"},
		},
		{
			name:          "short warning",
			reason:        "System idle",
			timeRemaining: 30 * time.Second,
			wantContains:  []string{"30 seconds", "System idle", "cancel"},
		},
		{
			name:          "warning with mixed time",
			reason:        "Disconnected session",
			timeRemaining: 2*time.Minute + 30*time.Second,
			wantContains:  []string{"2 minutes 30 seconds", "Disconnected session", "cancel"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatWarningMessage(tt.reason, tt.timeRemaining)

			// Check that all expected substrings are present
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("FormatWarningMessage() = %q, want to contain %q", got, want)
				}
			}

			// Verify it's a complete sentence
			if !strings.Contains(got, ".") {
				t.Errorf("FormatWarningMessage() = %q, should contain a period", got)
			}
		})
	}
}

// TestFormatCancellationMessage tests the cancellation message
func TestFormatCancellationMessage(t *testing.T) {
	got := FormatCancellationMessage()

	// Should be a non-empty message
	if got == "" {
		t.Error("FormatCancellationMessage() returned empty string")
	}

	// Should mention cancellation and activity
	if !strings.Contains(strings.ToLower(got), "cancel") {
		t.Errorf("FormatCancellationMessage() = %q, should mention cancellation", got)
	}

	if !strings.Contains(strings.ToLower(got), "activity") {
		t.Errorf("FormatCancellationMessage() = %q, should mention activity", got)
	}
}

// TestFormatWarningMessageStructure validates the warning message structure
func TestFormatWarningMessageStructure(t *testing.T) {
	reason := "Test reason"
	timeRemaining := 3 * time.Minute

	msg := FormatWarningMessage(reason, timeRemaining)

	// Message should have reasonable length
	if len(msg) < 20 {
		t.Errorf("FormatWarningMessage() too short: %q", msg)
	}

	// Should contain the formatted time
	timeStr := FormatTimeRemaining(timeRemaining)
	if !strings.Contains(msg, timeStr) {
		t.Errorf("FormatWarningMessage() = %q, should contain formatted time %q", msg, timeStr)
	}

	// Should contain the reason
	if !strings.Contains(msg, reason) {
		t.Errorf("FormatWarningMessage() = %q, should contain reason %q", msg, reason)
	}
}
