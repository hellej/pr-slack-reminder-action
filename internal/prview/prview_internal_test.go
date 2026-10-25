package prview

import (
	"testing"
	"time"
)

// 2026-10-09 is a Friday.
func TestWeekdayTimeBetween(t *testing.T) {
	utcPlus3 := time.FixedZone("UTC+3", 3*60*60)
	utcMinus5 := time.FixedZone("UTC-5", -5*60*60)
	at := func(day, hour int) time.Time { return time.Date(2026, time.October, day, hour, 0, 0, 0, time.UTC) }
	tests := []struct {
		name     string
		from     time.Time
		to       time.Time
		expected time.Duration
	}{
		{name: "within one weekday", from: at(6, 9), to: at(6, 17).Add(30 * time.Minute), expected: 8*time.Hour + 30*time.Minute},
		{name: "over a whole weekday", from: at(6, 0), to: at(7, 0), expected: 24 * time.Hour},
		{name: "Friday evening to Monday morning", from: at(9, 18), to: at(12, 6), expected: 12 * time.Hour},
		{name: "from Saturday to Monday", from: at(10, 12), to: at(12, 9), expected: 9 * time.Hour},
		{name: "Friday night to Sunday", from: at(9, 22), to: at(11, 15), expected: 2 * time.Hour},
		{name: "Friday to Saturday 00:00", from: at(9, 23), to: at(10, 0), expected: time.Hour},
		{name: "Friday to Monday 00:00", from: at(9, 23), to: at(12, 0), expected: time.Hour},
		{name: "Saturday 00:00 to Monday 00:00", from: at(10, 0), to: at(12, 0), expected: 0},
		{name: "Monday 00:00 onwards", from: at(12, 0), to: at(12, 1), expected: time.Hour},
		{name: "start and end inside one weekend", from: at(10, 10), to: at(11, 20), expected: 0},
		{name: "over two weekends", from: at(2, 12), to: at(12, 12), expected: 144 * time.Hour},
		{name: "from equal to to", from: at(6, 9), to: at(6, 9), expected: 0},
		{name: "from after to on an earlier day", from: at(7, 9), to: at(6, 9), expected: 0},
		{name: "from after to on the same day", from: at(6, 10), to: at(6, 9), expected: 0},
		{
			name:     "local Saturday that is a UTC Friday counts",
			from:     time.Date(2026, time.October, 10, 1, 0, 0, 0, utcPlus3),
			to:       at(10, 12),
			expected: 2 * time.Hour,
		},
		{
			name:     "local Sunday that is a UTC Monday counts",
			from:     time.Date(2026, time.October, 11, 22, 0, 0, 0, utcMinus5),
			to:       at(12, 9),
			expected: 6 * time.Hour,
		},
		{
			name:     "local Monday that is a UTC Sunday does not count",
			from:     at(9, 23),
			to:       time.Date(2026, time.October, 12, 2, 0, 0, 0, utcPlus3),
			expected: time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WeekdayTimeBetween(tt.from, tt.to); got != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}
