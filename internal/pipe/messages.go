//go:build windows

package pipe

import (
	"fmt"
	"time"
)

// FormatWarningMessage creates a warning notification message
func FormatWarningMessage(reason string, timeRemaining time.Duration) string {
	timeStr := FormatTimeRemaining(timeRemaining)
	return fmt.Sprintf("This VM will hibernate in %s.\n\n%s\n\nMove your mouse or press a key to cancel.", timeStr, reason)
}

// FormatCancellationMessage creates a cancellation notification message
func FormatCancellationMessage() string {
	return "Hibernation canceled due to user activity."
}

// FormatTimeRemaining formats a duration into a friendly string rounded to nearest 30 seconds
func FormatTimeRemaining(d time.Duration) string {
	totalSeconds := int(d.Seconds())

	// Handle zero or negative duration
	if totalSeconds <= 0 {
		return "immediately"
	}

	// Round to nearest 30 seconds
	rounded := ((totalSeconds + 15) / 30) * 30

	if rounded < 30 {
		return "less than 30 seconds"
	}

	minutes := rounded / 60
	seconds := rounded % 60

	if minutes > 0 && seconds > 0 {
		// Both minutes and seconds - handle pluralization
		minWord := "minute"
		if minutes > 1 {
			minWord = "minutes"
		}
		secWord := "second"
		if seconds > 1 {
			secWord = "seconds"
		}
		return fmt.Sprintf("%d %s %d %s", minutes, minWord, seconds, secWord)
	} else if minutes > 0 {
		if minutes == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", minutes)
	} else {
		return "30 seconds"
	}
}
