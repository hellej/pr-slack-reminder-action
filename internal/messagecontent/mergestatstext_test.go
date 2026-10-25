package messagecontent

import (
	"testing"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/internal/mergestats"
)

func durationPtr(duration time.Duration) *time.Duration {
	return &duration
}

const sevenDays = 7 * 24 * time.Hour

func mergeStatsTextOverWindowEndingAtGeneratedAt(window time.Duration, stats mergestats.MergeStats) string {
	return mergeStatsTextAgainstTheWeekBefore(window, stats, nil)
}

func mergeStatsTextAgainstTheWeekBefore(
	window time.Duration, stats mergestats.MergeStats, weekBefore *mergestats.MergeStats,
) string {
	stats.ComputedAt = generatedAt
	stats.MergedSince = generatedAt.Add(-window)
	return GetContent(nil, nil, nil, &stats, weekBefore, time.Time{}, generatedAt, config.ContentInputs{}).MergeStatsText
}

func TestGetContentLeavesTheMergeStatsTextEmptyWithoutStats(t *testing.T) {
	content := GetContent(nil, nil, nil, nil, nil, time.Time{}, generatedAt, config.ContentInputs{})

	if content.MergeStatsText != "" {
		t.Errorf("expected no merge stats text, got %q", content.MergeStatsText)
	}
}

func TestGetContentMergeStatsText(t *testing.T) {
	tests := []struct {
		name     string
		stats    mergestats.MergeStats
		expected string
	}{
		{
			name: "both medians",
			stats: mergestats.MergeStats{
				MergedCount:             11,
				MedianTimeToFirstReview: durationPtr(3*time.Hour + 42*time.Minute),
				MedianTimeToMerge:       durationPtr(31*time.Hour + 12*time.Minute),
			},
			expected: "Last 7 days: 11 merged · median 3.7 hours to first review, 1.3 days to merge",
		},
		{
			name:     "no merged PR had a first review",
			stats:    mergestats.MergeStats{MergedCount: 2, MedianTimeToMerge: durationPtr(24 * time.Hour)},
			expected: "Last 7 days: 2 merged · median 1.0 days to merge",
		},
		{
			name:     "nothing merged",
			stats:    mergestats.MergeStats{},
			expected: "Last 7 days: 0 merged",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if text := mergeStatsTextOverWindowEndingAtGeneratedAt(sevenDays, tt.stats); text != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, text)
			}
		})
	}
}

func TestGetContentMergeStatsDurationText(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{duration: 0, expected: "0 minutes"},
		{duration: time.Minute, expected: "1 minute"},
		{duration: 24*time.Minute + 29*time.Second, expected: "24 minutes"},
		{duration: 59*time.Minute + 36*time.Second, expected: "60 minutes"},
		{duration: time.Hour, expected: "1.0 hours"},
		{duration: 4*time.Hour + 2*time.Minute, expected: "4.0 hours"},
		{duration: 23*time.Hour + 57*time.Minute + 36*time.Second, expected: "24.0 hours"},
		{duration: 24 * time.Hour, expected: "1.0 days"},
		{duration: 60 * time.Hour, expected: "2.5 days"},
	}

	for _, tt := range tests {
		t.Run(tt.duration.String(), func(t *testing.T) {
			text := mergeStatsTextOverWindowEndingAtGeneratedAt(sevenDays, mergestats.MergeStats{
				MergedCount: 1, MedianTimeToMerge: durationPtr(tt.duration),
			})

			expected := "Last 7 days: 1 merged · median " + tt.expected + " to merge"
			if text != expected {
				t.Errorf("expected %q, got %q", expected, text)
			}
		})
	}
}

func TestGetContentMergeStatsWindowText(t *testing.T) {
	tests := []struct {
		name     string
		window   time.Duration
		expected string
	}{
		{name: "under 12 hours", window: 11 * time.Hour, expected: "Last 1 day: 2 merged"},
		{name: "exactly 1 day", window: 24 * time.Hour, expected: "Last 1 day: 2 merged"},
		{name: "2.4 days", window: 57*time.Hour + 36*time.Minute, expected: "Last 2 days: 2 merged"},
		{name: "2.6 days", window: 62*time.Hour + 24*time.Minute, expected: "Last 3 days: 2 merged"},
		{name: "7 days", window: sevenDays, expected: "Last 7 days: 2 merged"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if text := mergeStatsTextOverWindowEndingAtGeneratedAt(tt.window, mergestats.MergeStats{MergedCount: 2}); text != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, text)
			}
		})
	}
}

func TestGetContentMergeStatsTextAgainstTheWeekBefore(t *testing.T) {
	tests := []struct {
		name       string
		stats      mergestats.MergeStats
		weekBefore mergestats.MergeStats
		expected   string
	}{
		{
			name: "both medians with a change",
			stats: mergestats.MergeStats{
				MergedCount:             11,
				MedianTimeToFirstReview: durationPtr(3*time.Hour + 42*time.Minute),
				MedianTimeToMerge:       durationPtr(31*time.Hour + 12*time.Minute),
			},
			weekBefore: mergestats.MergeStats{
				MergedCount:             6,
				MedianTimeToFirstReview: durationPtr(4 * time.Hour),
				MedianTimeToMerge:       durationPtr(24 * time.Hour),
			},
			expected: "Last 7 days: 11 merged · median 3.7 hours to first review, 1.3 days to merge (-8 %, +30 %)",
		},
		{
			name:  "the merge median alone, with a change",
			stats: mergestats.MergeStats{MergedCount: 2, MedianTimeToMerge: durationPtr(24 * time.Hour)},
			weekBefore: mergestats.MergeStats{
				MergedCount:             5,
				MedianTimeToFirstReview: durationPtr(time.Hour),
				MedianTimeToMerge:       durationPtr(30 * time.Hour),
			},
			expected: "Last 7 days: 2 merged · median 1.0 days to merge (-20 %)",
		},
		{
			name:       "a small drop rounding to no change reads +0 %",
			stats:      mergestats.MergeStats{MergedCount: 2, MedianTimeToMerge: durationPtr(999 * time.Hour)},
			weekBefore: mergestats.MergeStats{MergedCount: 2, MedianTimeToMerge: durationPtr(1000 * time.Hour)},
			expected:   "Last 7 days: 2 merged · median 41.6 days to merge (+0 %)",
		},
		{
			name: "a shown median without a change leaves the parenthetical out",
			stats: mergestats.MergeStats{
				MergedCount:             3,
				MedianTimeToFirstReview: durationPtr(2 * time.Hour),
				MedianTimeToMerge:       durationPtr(24 * time.Hour),
			},
			weekBefore: mergestats.MergeStats{MergedCount: 4, MedianTimeToMerge: durationPtr(30 * time.Hour)},
			expected:   "Last 7 days: 3 merged · median 2.0 hours to first review, 1.0 days to merge",
		},
		{
			name: "a week-before median of 0 leaves the parenthetical out",
			stats: mergestats.MergeStats{
				MergedCount:             3,
				MedianTimeToFirstReview: durationPtr(2 * time.Hour),
				MedianTimeToMerge:       durationPtr(24 * time.Hour),
			},
			weekBefore: mergestats.MergeStats{
				MergedCount:             4,
				MedianTimeToFirstReview: durationPtr(0),
				MedianTimeToMerge:       durationPtr(30 * time.Hour),
			},
			expected: "Last 7 days: 3 merged · median 2.0 hours to first review, 1.0 days to merge",
		},
		{
			name:  "nothing merged",
			stats: mergestats.MergeStats{},
			weekBefore: mergestats.MergeStats{
				MergedCount:             4,
				MedianTimeToFirstReview: durationPtr(time.Hour),
				MedianTimeToMerge:       durationPtr(30 * time.Hour),
			},
			expected: "Last 7 days: 0 merged",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if text := mergeStatsTextAgainstTheWeekBefore(sevenDays, tt.stats, &tt.weekBefore); text != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, text)
			}
		})
	}
}
