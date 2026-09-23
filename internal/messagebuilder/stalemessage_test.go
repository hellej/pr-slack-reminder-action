package messagebuilder_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hellej/pr-slack-reminder-action/internal/messagebuilder"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

// Blocks as MarshalSentBlocks records them: Go's json.Marshal output, so `<` is \u003c and
// emoji are raw UTF-8.
var sentContentBlocks = []string{
	`{"type":"header","text":{"type":"plain_text","text":"👀 Waiting for review","emoji":true},"block_id":"heading_waiting_for_review","level":2}`,
	`{"type":"rich_text","block_id":"section_waiting_for_review_repository_1","elements":[{"type":"rich_text_section","elements":[{"type":"link","url":"https://github.com/test-org/repo-two/pulls","text":"repo-two","style":{"bold":true}}]},{"type":"rich_text_list","elements":[{"type":"rich_text_section","elements":[{"type":"link","url":"https://github.com/test-org/repo-two/pull/42","text":"Old PR past the threshold","style":{"bold":true}},{"type":"text","text":" 🚨 ","style":{}},{"type":"text","text":"3 days old","style":{"bold":true,"code":true}},{"type":"text","text":" by ","style":{}},{"type":"user","user_id":"U2234567890","style":{}}]}],"style":"bullet","indent":0,"border":0,"offset":0}]}`,
	`{"type":"section","text":{"type":"mrkdwn","text":" "}}`,
	`{"type":"rich_text","block_id":"section_waiting_for_review_repository_2","elements":[{"type":"rich_text_section","elements":[{"type":"link","url":"https://github.com/test-org/repo-one/pulls","text":"repo-one","style":{"bold":true}}]},{"type":"rich_text_list","elements":[{"type":"rich_text_section","elements":[{"type":"link","url":"https://github.com/test-org/repo-one/pull/11","text":"Add pagination to the PR listing","style":{"bold":true}},{"type":"text","text":" 2 hours ago","style":{"italic":true}},{"type":"text","text":" by ","style":{}},{"type":"text","text":"Alice Anderson","style":{}}]}],"style":"bullet","indent":0,"border":0,"offset":0}]}`,
}

const sentLiveFooterBlock = `{"type":"context","elements":[{"type":"mrkdwn","text":"_Live, updated \u003c!date^1789819920^{time}|12:12 UTC\u003e_"}]}`

// generatedAt is 2026-09-19 12:12 UTC
const expectedStaleFooterBlock = `{"type":"context","elements":[{"type":"mrkdwn","text":"_⚠️ Stale, updated \u003c!date^1789819920^{date_pretty} at {time}|Sep 19 12:12 UTC\u003e_"}]}`

func sentMessageBlocks() json.RawMessage {
	return json.RawMessage("[" + strings.Join(append(sentContentBlocks, sentLiveFooterBlock), ",") + "]")
}

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

func assertStaleBlocks(t *testing.T, actual []string, expected []string) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("Expected %d blocks, got %d:\n%s", len(expected), len(actual), strings.Join(actual, "\n"))
	}
	for index := range expected {
		if actual[index] != expected[index] {
			t.Errorf("Block %d:\nexpected %s\ngot      %s", index, expected[index], actual[index])
		}
	}
}

func TestStaleMessageResendsTheContentBlocksAsSentAndSwapsTheFooter(t *testing.T) {
	actual := buildAndMarshalStaleMessage(t, sentMessageBlocks())

	assertStaleBlocks(t, actual, append(sentContentBlocks, expectedStaleFooterBlock))
}

// state.Save leaves the stored blocks indented
func TestStaleMessageFromIndentedBlocksResendsThemCompact(t *testing.T) {
	var indentedBlocks bytes.Buffer
	if err := json.Indent(&indentedBlocks, sentMessageBlocks(), "", "  "); err != nil {
		t.Fatalf("Failed to indent the fixture: %v", err)
	}

	actual := buildAndMarshalStaleMessage(t, indentedBlocks.Bytes())

	assertStaleBlocks(t, actual, append(sentContentBlocks, expectedStaleFooterBlock))
}

func TestStaleMessageOfAFooterOnlyMessageIsTheStaleFooter(t *testing.T) {
	actual := buildAndMarshalStaleMessage(t, json.RawMessage("["+sentLiveFooterBlock+"]"))

	assertStaleBlocks(t, actual, []string{expectedStaleFooterBlock})
}

func TestStaleMessageErrors(t *testing.T) {
	testCases := []struct {
		name       string
		sentBlocks json.RawMessage
	}{
		{name: "no bytes", sentBlocks: nil},
		{name: "JSON null", sentBlocks: json.RawMessage(`null`)},
		{name: "empty block array", sentBlocks: json.RawMessage(`[]`)},
		{name: "a block object, not an array", sentBlocks: json.RawMessage(`{"type":"divider"}`)},
		{name: "truncated JSON", sentBlocks: json.RawMessage(`[{"type":"divider"},`)},
		{
			name:       "a content block without a type",
			sentBlocks: json.RawMessage(`[{"text":"no type"},` + sentLiveFooterBlock + `]`),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := messagebuilder.BuildStaleMessage(tc.sentBlocks, generatedAt); err == nil {
				t.Error("Expected an error, got nil")
			}
		})
	}
}
