package testhelpers

import "time"

// WeekdayTimeBefore returns the latest time from which instant is weekdayTime away, counting only
// Monday to Friday, UTC: the inverse of how prview ages a PR. A fixture aged with it renders the
// same age whatever day the suite runs on.
func WeekdayTimeBefore(instant time.Time, weekdayTime time.Duration) time.Time {
	cursor := instant.UTC()
	remaining := weekdayTime
	for remaining > 0 {
		dayStart := startOfDayEndingAt(cursor)
		if isWeekend(dayStart) {
			cursor = dayStart
			continue
		}
		if remaining <= cursor.Sub(dayStart) {
			return cursor.Add(-remaining)
		}
		remaining -= cursor.Sub(dayStart)
		cursor = dayStart
	}
	return cursor
}

// startOfDayEndingAt returns the start of the UTC day holding the moment just before cursor, so a
// cursor at midnight belongs to the day it ends.
func startOfDayEndingAt(cursor time.Time) time.Time {
	return cursor.Add(-time.Nanosecond).Truncate(24 * time.Hour)
}

func isWeekend(day time.Time) bool {
	return day.Weekday() == time.Saturday || day.Weekday() == time.Sunday
}
