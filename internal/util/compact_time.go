package util

import (
	"fmt"
	"time"
)

// CompactTime prints one time representation, never both a date and an age.
// Absolute dates use UTC; future timestamps are not described as past events.
func CompactTime(value, now time.Time) string {
	age := now.Sub(value)
	if age >= 0 && age < 24*time.Hour {
		switch {
		case age < time.Minute:
			return "now"
		case age < time.Hour:
			return fmt.Sprintf("%dm ago", int64(age/time.Minute))
		default:
			return fmt.Sprintf("%dh ago", int64(age/time.Hour))
		}
	}
	value = value.UTC()
	if value.Year() != now.UTC().Year() {
		return value.Format("2 Jan 2006 15:04 UTC")
	}
	return value.Format("2 Jan 15:04 UTC")
}
