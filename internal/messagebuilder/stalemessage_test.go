package messagebuilder_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hellej/pr-slack-reminder-action/internal/messagebuilder"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

const sentLiveFooterBlock = `{"type":"context","elements":[{"type":"mrkdwn","text":"_Live, updated \u003c!date^1789819920^{time}|12:12 UTC\u003e_"}]}`

// generatedAt is 2026-09-19 12:12 UTC
const expectedStaleFooterBlock = `{"type":"context","elements":[{"type":"mrkdwn","text":"_Updated \u003c!date^1789819920^{date_pretty} at {time}|Sep 19 12:12 UTC\u003e_"}]}`

const expectedStaleLineBlock = `{"type":"context","elements":[{"type":"mrkdwn","text":"_⚠️ Stale, updated \u003c!date^1789819920^{date_pretty} at {time}|Sep 19 12:12 UTC\u003e_"}]}`

func buildAndMarshalStaleMessage(t *testing.T, sentBlocks json.RawMessage) []string {
	t.Helper()
	message, err := messagebuilder.BuildStaleMessage(sentBlocks, generatedAt)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	marshalledBlocks, err := json.Marshal(message.Blocks.BlockSet)
	if err != nil {
		t.Fatalf("Failed to marshal the stale message: %v", err)
	}
	var blocks []json.RawMessage
	if err := json.Unmarshal(marshalledBlocks, &blocks); err != nil {
		t.Fatalf("Failed to split the stale message's blocks: %v", err)
	}
	return utilities.Map(blocks, func(block json.RawMessage) string { return string(block) })
}

func dividerBlocks(count int) []string {
	blocks := make([]string, count)
	for index := range blocks {
		blocks[index] = fmt.Sprintf(`{"type":"divider","block_id":"divider_%d"}`, index)
	}
	return blocks
}

// BuildMessage caps a message at 50 blocks, footer included. The stale message adds a block, so
// a full one has to give up a content block.
func TestStaleMessageStaysWithinTheBlockCap(t *testing.T) {
	testCases := []struct {
		name               string
		sentContentBlocks  int
		expectedLastKeptID string
	}{
		{
			name:               "49 sent blocks keep every content block",
			sentContentBlocks:  48,
			expectedLastKeptID: "divider_47",
		},
		{
			name:               "50 sent blocks drop the last content block",
			sentContentBlocks:  49,
			expectedLastKeptID: "divider_47",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sentBlocks := append(dividerBlocks(tc.sentContentBlocks), sentLiveFooterBlock)

			actual := buildAndMarshalStaleMessage(t, json.RawMessage("["+strings.Join(sentBlocks, ",")+"]"))

			if len(actual) != 50 {
				t.Fatalf("Expected 50 blocks, got %d", len(actual))
			}
			if actual[0] != expectedStaleLineBlock || actual[49] != expectedStaleFooterBlock {
				t.Errorf("Expected the stale line first and the stale footer last, got %s and %s", actual[0], actual[49])
			}
			lastKept := fmt.Sprintf(`{"type":"divider","block_id":"%s"}`, tc.expectedLastKeptID)
			if actual[48] != lastKept {
				t.Errorf("Expected the last content block %s, got %s", lastKept, actual[48])
			}
		})
	}
}

// Stored blocks that are JSON null or an empty array have no footer to drop.
func TestStaleMessageErrorsOnNoSentBlocks(t *testing.T) {
	for _, sentBlocks := range []string{`null`, `[]`} {
		t.Run(sentBlocks, func(t *testing.T) {
			if _, err := messagebuilder.BuildStaleMessage(json.RawMessage(sentBlocks), generatedAt); err == nil {
				t.Error("Expected an error, got nil")
			}
		})
	}
}
