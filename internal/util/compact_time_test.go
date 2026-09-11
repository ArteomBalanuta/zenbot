package util

import (
	"testing"
	"time"
)

func TestCompactTimeBoundaries(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name  string
		value time.Time
		want  string
	}{
		{"now", now, "now"},
		{"seconds", now.Add(-59 * time.Second), "now"},
		{"minute", now.Add(-time.Minute), "1m ago"},
		{"minutes", now.Add(-59 * time.Minute), "59m ago"},
		{"hour", now.Add(-time.Hour), "1h ago"},
		{"hours", now.Add(-23 * time.Hour), "23h ago"},
		{"day-old-previous-year", now.Add(-24 * time.Hour), "31 Dec 2025 12:00 UTC"},
		{"same-year", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "12h ago"},
		{"future", now.Add(time.Minute), "1 Jan 12:01 UTC"},
		{"utc", time.Date(2025, 12, 1, 15, 4, 59, 0, time.FixedZone("east", 3*3600)), "1 Dec 2025 12:04 UTC"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := CompactTime(tc.value, now); got != tc.want {
				t.Fatalf("got=%q want=%q", got, tc.want)
			}
		})
	}
}
