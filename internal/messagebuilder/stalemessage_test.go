package messagebuilder_test

import (
	"encoding/json"
	"testing"

	"github.com/hellej/pr-slack-reminder-action/internal/messagebuilder"
)

func TestStaleMessageErrorsOnNullOrEmptySentBlocks(t *testing.T) {
	for _, sentBlocks := range []string{`null`, `[]`} {
		t.Run(sentBlocks, func(t *testing.T) {
			if _, err := messagebuilder.BuildStaleMessage(json.RawMessage(sentBlocks), generatedAt); err == nil {
				t.Error("Expected an error, got nil")
			}
		})
	}
}
