package messagebuilder_test

import (
	"encoding/json"
	"testing"

	"github.com/hellej/pr-slack-reminder-action/internal/messagebuilder"
)

func TestMessageMarkedStaleErrorsOnSentBlocksThatAreNotABlockList(t *testing.T) {
	for _, sentBlocks := range []string{`null`, `[]`, `{"type":"divider"}`} {
		t.Run(sentBlocks, func(t *testing.T) {
			if _, err := messagebuilder.BuildMessageMarkedStale(json.RawMessage(sentBlocks), generatedAt); err == nil {
				t.Error("Expected an error, got nil")
			}
		})
	}
}
