package messagebuilder_test

import (
	"testing"
	"time"

	"github.com/slack-go/slack"

	"github.com/hellej/pr-slack-reminder-action/internal/mergestats"
	"github.com/hellej/pr-slack-reminder-action/internal/messagebuilder"
	"github.com/hellej/pr-slack-reminder-action/internal/messagecontent"
	"github.com/hellej/pr-slack-reminder-action/internal/prview"
)

func durationPtr(duration time.Duration) *time.Duration {
	return &duration
}

func mergeStatsLineText(t *testing.T, message slack.Message) string {
	t.Helper()
	blocks := message.Blocks.BlockSet
	if len(blocks) == 0 {
		t.Fatal("expected the merge stats line last, got no blocks")
	}
	line, isContextBlock := blocks[len(blocks)-1].(*slack.ContextBlock)
	if !isContextBlock || line.BlockID != "merge_stats_line" {
		t.Fatalf("expected the merge stats line last, got blocks %v", blockIDs(blocks))
	}
	if len(line.ContextElements.Elements) != 1 {
		t.Fatalf("expected one element in the merge stats line, got %d", len(line.ContextElements.Elements))
	}
	text := line.ContextElements.Elements[0].(*slack.TextBlockObject)
	if text.Type != "mrkdwn" {
		t.Errorf("expected an mrkdwn element, got %q", text.Type)
	}
	return text.Text
}

func statsOverWindowEndingAtGeneratedAt(window time.Duration, stats mergestats.MergeStats) *mergestats.MergeStats {
	stats.ComputedAt = generatedAt
	stats.MergedSince = generatedAt.Add(-window)
	return &stats
}

const sevenDays = 7 * 24 * time.Hour

func TestMergeStatsLineText(t *testing.T) {
	tests := []struct {
		name       string
		stats      mergestats.MergeStats
		weekBefore *mergestats.MergeStats
		expected   string
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
		{
			name: "both medians with a change",
			stats: mergestats.MergeStats{
				MergedCount:             11,
				MedianTimeToFirstReview: durationPtr(3*time.Hour + 42*time.Minute),
				MedianTimeToMerge:       durationPtr(31*time.Hour + 12*time.Minute),
			},
			weekBefore: &mergestats.MergeStats{
				MergedCount:             6,
				MedianTimeToFirstReview: durationPtr(4 * time.Hour),
				MedianTimeToMerge:       durationPtr(24 * time.Hour),
			},
			expected: "Last 7 days: 11 merged · median 3.7 hours to first review, 1.3 days to merge (-8 %, +30 %)",
		},
		{
			name:  "the merge median alone, with a change",
			stats: mergestats.MergeStats{MergedCount: 2, MedianTimeToMerge: durationPtr(24 * time.Hour)},
			weekBefore: &mergestats.MergeStats{
				MergedCount:             5,
				MedianTimeToFirstReview: durationPtr(time.Hour),
				MedianTimeToMerge:       durationPtr(30 * time.Hour),
			},
			expected: "Last 7 days: 2 merged · median 1.0 days to merge (-20 %)",
		},
		{
			name:       "a small drop reads 0 %",
			stats:      mergestats.MergeStats{MergedCount: 2, MedianTimeToMerge: durationPtr(999 * time.Hour)},
			weekBefore: &mergestats.MergeStats{MergedCount: 2, MedianTimeToMerge: durationPtr(1000 * time.Hour)},
			expected:   "Last 7 days: 2 merged · median 41.6 days to merge (0 %)",
		},
		{
			name: "a shown median without a change leaves the parenthetical out",
			stats: mergestats.MergeStats{
				MergedCount:             3,
				MedianTimeToFirstReview: durationPtr(2 * time.Hour),
				MedianTimeToMerge:       durationPtr(24 * time.Hour),
			},
			weekBefore: &mergestats.MergeStats{MergedCount: 4, MedianTimeToMerge: durationPtr(30 * time.Hour)},
			expected:   "Last 7 days: 3 merged · median 2.0 hours to first review, 1.0 days to merge",
		},
		{
			name: "a week-before median of 0 leaves the parenthetical out",
			stats: mergestats.MergeStats{
				MergedCount:             3,
				MedianTimeToFirstReview: durationPtr(2 * time.Hour),
				MedianTimeToMerge:       durationPtr(24 * time.Hour),
			},
			weekBefore: &mergestats.MergeStats{
				MergedCount:             4,
				MedianTimeToFirstReview: durationPtr(0),
				MedianTimeToMerge:       durationPtr(30 * time.Hour),
			},
			expected: "Last 7 days: 3 merged · median 2.0 hours to first review, 1.0 days to merge",
		},
		{
			name:  "nothing merged, with a week before",
			stats: mergestats.MergeStats{},
			weekBefore: &mergestats.MergeStats{
				MergedCount:             4,
				MedianTimeToFirstReview: durationPtr(time.Hour),
				MedianTimeToMerge:       durationPtr(30 * time.Hour),
			},
			expected: "Last 7 days: 0 merged",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message, _ := messagebuilder.BuildMessageToPost(messagecontent.Content{
				MergeStats:           statsOverWindowEndingAtGeneratedAt(sevenDays, tt.stats),
				MergeStatsWeekBefore: tt.weekBefore,
				GeneratedAt:          generatedAt,
			})

			if text := mergeStatsLineText(t, message); text != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, text)
			}
		})
	}
}

func TestMergeStatsLineDurationText(t *testing.T) {
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
			message, _ := messagebuilder.BuildMessageToPost(messagecontent.Content{
				MergeStats: statsOverWindowEndingAtGeneratedAt(sevenDays, mergestats.MergeStats{
					MergedCount: 1, MedianTimeToMerge: durationPtr(tt.duration),
				}),
				GeneratedAt: generatedAt,
			})

			expected := "Last 7 days: 1 merged · median " + tt.expected + " to merge"
			if text := mergeStatsLineText(t, message); text != expected {
				t.Errorf("expected %q, got %q", expected, text)
			}
		})
	}
}

func TestMergeStatsLineWindowText(t *testing.T) {
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
			message, _ := messagebuilder.BuildMessageToPost(messagecontent.Content{
				MergeStats:  statsOverWindowEndingAtGeneratedAt(tt.window, mergestats.MergeStats{MergedCount: 2}),
				GeneratedAt: generatedAt,
			})

			if text := mergeStatsLineText(t, message); text != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, text)
			}
		})
	}
}

func TestMergeStatsLineSitsAfterTheSectionsAndBeforeTheUpdateTimeFooter(t *testing.T) {
	content := messagecontent.Content{
		WaitingForReview: prview.PRSection{Rows: []prview.Row{testPR(testPROptions{title: "Open PR"})}},
		MergeStats:       &mergestats.MergeStats{MergedCount: 3},
		GeneratedAt:      generatedAt,
	}

	postedMessage, _ := messagebuilder.BuildMessageToPost(content)
	editedMessage, _ := messagebuilder.BuildMessageWithUpdateTimeFooter(content)

	assertBlockIDs(t, postedMessage, []string{
		"heading_waiting_for_review", "section_waiting_for_review", "merge_stats_line",
	})
	assertBlockIDs(t, editedMessage, []string{
		"heading_waiting_for_review", "section_waiting_for_review", "merge_stats_line", "update_time_footer",
	})
}
