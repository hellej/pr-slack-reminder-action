package messagebuilder_test

import (
	"encoding/json"
	"testing"

	"github.com/hellej/pr-slack-reminder-action/internal/messagebuilder"
)

// Stored blocks that are JSON null or an empty array hold no content to mark stale.
func TestStaleMessageErrorsOnNoSentBlocks(t *testing.T) {
	for _, sentBlocks := range []string{`null`, `[]`} {
		t.Run(sentBlocks, func(t *testing.T) {
			if _, err := messagebuilder.BuildStaleMessage(json.RawMessage(sentBlocks), generatedAt); err == nil {
				t.Error("Expected an error, got nil")
			}
		})
	}
}
