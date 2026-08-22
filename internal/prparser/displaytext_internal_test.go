package prparser

import (
	"testing"
	"time"
)

func TestDurationText(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{name: "zero", duration: 0, expected: "0 minutes"},
		{name: "minutes", duration: 30 * time.Minute, expected: "30 minutes"},
		{name: "minutes rounded up", duration: 30*time.Minute + 40*time.Second, expected: "31 minutes"},
		{name: "just under an hour", duration: 59 * time.Minute, expected: "59 minutes"},
		{name: "exactly an hour", duration: time.Hour, expected: "1 hours"},
		{name: "hours rounded", duration: 90 * time.Minute, expected: "2 hours"},
		{name: "just under a day rounds to 24 hours", duration: 23*time.Hour + 59*time.Minute, expected: "24 hours"},
		{name: "exactly a day", duration: 24 * time.Hour, expected: "1 days"},
		{name: "days", duration: 72 * time.Hour, expected: "3 days"},
		{name: "days truncated after rounding to hours", duration: 47 * time.Hour, expected: "1 days"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := durationText(tt.duration); got != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, got)
			}
		})
	}
}
