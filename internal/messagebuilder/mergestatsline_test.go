package messagebuilder_test

import (
	"testing"

	"github.com/slack-go/slack"

	"github.com/hellej/pr-slack-reminder-action/internal/messagebuilder"
	"github.com/hellej/pr-slack-reminder-action/internal/messagecontent"
	"github.com/hellej/pr-slack-reminder-action/internal/prview"
)

const testMergeStatsText = "Last 7 days: 3 merged"

func TestMergeStatsLineIsOneMrkdwnElementHoldingTheGivenText(t *testing.T) {
	message, _ := messagebuilder.BuildMessageToPost(messagecontent.Content{
		MergeStatsText: "Last 2 days: 5 merged · median 1.0 days to merge",
		GeneratedAt:    generatedAt,
	})

	assertBlockIDs(t, message, []string{"merge_stats_line"})
	line := message.Blocks.BlockSet[0].(*slack.ContextBlock)
	if len(line.ContextElements.Elements) != 1 {
		t.Fatalf("expected one element in the merge stats line, got %d", len(line.ContextElements.Elements))
	}
	text := line.ContextElements.Elements[0].(*slack.TextBlockObject)
	if text.Type != "mrkdwn" || text.Text != "Last 2 days: 5 merged · median 1.0 days to merge" {
		t.Errorf("expected the given text as mrkdwn, got %q as %q", text.Text, text.Type)
	}
}

func TestEmptyMergeStatsTextRendersNoLine(t *testing.T) {
	content := messagecontent.Content{
		WaitingForReview: prview.PRSection{Rows: []prview.Row{testPR(testPROptions{title: "Open PR"})}},
		GeneratedAt:      generatedAt,
	}

	postedMessage, _ := messagebuilder.BuildMessageToPost(content)
	editedMessage, _ := messagebuilder.BuildMessageWithUpdateTimeFooter(content)

	assertBlockIDs(t, postedMessage, []string{"heading_waiting_for_review", "section_waiting_for_review"})
	assertBlockIDs(t, editedMessage, []string{
		"heading_waiting_for_review", "section_waiting_for_review", "update_time_footer",
	})
}

func TestMergeStatsLineSitsAfterTheSectionsAndBeforeTheUpdateTimeFooter(t *testing.T) {
	content := messagecontent.Content{
		WaitingForReview: prview.PRSection{Rows: []prview.Row{testPR(testPROptions{title: "Open PR"})}},
		MergeStatsText:   testMergeStatsText,
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
