package testhelpers

import (
	"testing"
	"time"
)

// 2026-10-09 is a Friday.
func TestWeekdayTimeBefore(t *testing.T) {
	utcPlus3 := time.FixedZone("UTC+3", 3*60*60)
	at := func(day, hour int) time.Time { return time.Date(2026, time.October, day, hour, 0, 0, 0, time.UTC) }
	tests := []struct {
		name        string
		instant     time.Time
		weekdayTime time.Duration
		expected    time.Time
	}{
		{name: "within one weekday", instant: at(6, 12), weekdayTime: 3 * time.Hour, expected: at(6, 9)},
		{name: "a whole weekday", instant: at(7, 0), weekdayTime: 24 * time.Hour, expected: at(6, 0)},
		{name: "Monday morning back to Friday evening", instant: at(12, 6), weekdayTime: 12 * time.Hour, expected: at(9, 18)},
		{name: "back to Monday 00:00 stops there", instant: at(12, 9), weekdayTime: 9 * time.Hour, expected: at(12, 0)},
		{name: "from Sunday back to Friday", instant: at(11, 15), weekdayTime: 2 * time.Hour, expected: at(9, 22)},
		{name: "from Saturday 00:00", instant: at(10, 0), weekdayTime: time.Hour, expected: at(9, 23)},
		{name: "from Monday 00:00", instant: at(12, 0), weekdayTime: time.Hour, expected: at(9, 23)},
		{name: "over two weekends", instant: at(12, 12), weekdayTime: 144 * time.Hour, expected: at(2, 12)},
		{name: "zero on a weekend", instant: at(11, 15), weekdayTime: 0, expected: at(11, 15)},
		{
			name:        "local Monday that is a UTC Sunday",
			instant:     time.Date(2026, time.October, 12, 2, 0, 0, 0, utcPlus3),
			weekdayTime: time.Hour,
			expected:    at(9, 23),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WeekdayTimeBefore(tt.instant, tt.weekdayTime); !got.Equal(tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}
