package main

import (
	"testing"
	"time"
)

// 2026-10-11 is a Sunday, 2026-10-12 a Monday.
func TestIsMondayInUTC(t *testing.T) {
	utcPlus3 := time.FixedZone("UTC+3", 3*60*60)
	utcMinus5 := time.FixedZone("UTC-5", -5*60*60)
	tests := []struct {
		name     string
		runStart time.Time
		expected bool
	}{
		{name: "Sunday 23:59 UTC", runStart: time.Date(2026, 10, 11, 23, 59, 0, 0, time.UTC)},
		{name: "Monday 00:00 UTC", runStart: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC), expected: true},
		{name: "Monday 23:59 UTC", runStart: time.Date(2026, 10, 12, 23, 59, 0, 0, time.UTC), expected: true},
		{name: "Tuesday 00:00 UTC", runStart: time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)},
		{name: "local Monday that is a UTC Sunday", runStart: time.Date(2026, 10, 12, 1, 0, 0, 0, utcPlus3)},
		{
			name:     "local Sunday that is a UTC Monday",
			runStart: time.Date(2026, 10, 11, 22, 0, 0, 0, utcMinus5),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMondayInUTC(tt.runStart); got != tt.expected {
				t.Errorf("isMondayInUTC(%v) = %t, expected %t", tt.runStart, got, tt.expected)
			}
		})
	}
}
