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

func TestMergeStatsLineText(t *testing.T) {
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
			message, _ := messagebuilder.BuildMessageToPost(messagecontent.Content{
				MergeStats:  &tt.stats,
				GeneratedAt: generatedAt,
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
				MergeStats:  &mergestats.MergeStats{MergedCount: 1, MedianTimeToMerge: durationPtr(tt.duration)},
				GeneratedAt: generatedAt,
			})

			expected := "Last 7 days: 1 merged · median " + tt.expected + " to merge"
			if text := mergeStatsLineText(t, message); text != expected {
				t.Errorf("expected %q, got %q", expected, text)
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
