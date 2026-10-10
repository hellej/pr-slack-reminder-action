package slackclient_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/slack-go/slack"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/slackclient"
	"github.com/hellej/pr-slack-reminder-action/testhelpers"
	"github.com/hellej/pr-slack-reminder-action/testhelpers/retryhelpers"
)

func TestGetAuthenticatedClient(t *testing.T) {
	client := slackclient.GetAuthenticatedClient("test-token")
	if client == nil {
		t.Fatal("Expected non-nil client, got nil")
	}
}

func TestGetChannelIDByName(t *testing.T) {
	tests := []struct {
		name                   string
		channelName            string
		publicChannels         []slack.Channel
		publicChannelsNextPage []slack.Channel
		privateChannels        []slack.Channel
		publicChannelsError    error
		privateChannelsError   error
		expectedChannelID      string
		expectedError          string
	}{
		{
			name:        "finds channel in public channels",
			channelName: "general",
			publicChannels: []slack.Channel{
				{GroupConversation: slack.GroupConversation{Name: "general", Conversation: slack.Conversation{ID: "C12345"}}},
			},
			expectedChannelID: "C12345",
		},
		{
			name:        "finds channel in private channels when public search doesn't find it",
			channelName: "private-team",
			publicChannels: []slack.Channel{
				{GroupConversation: slack.GroupConversation{Name: "other-public", Conversation: slack.Conversation{ID: "C11111"}}},
			},
			privateChannels: []slack.Channel{
				{GroupConversation: slack.GroupConversation{Name: "private-team", Conversation: slack.Conversation{ID: "C67890"}}},
			},
			expectedChannelID: "C67890",
		},
		{
			name:        "finds channel in public channels and doesn't need to check private",
			channelName: "general",
			publicChannels: []slack.Channel{
				{GroupConversation: slack.GroupConversation{Name: "general", Conversation: slack.Conversation{ID: "C12345"}}},
			},
			privateChannels: []slack.Channel{
				{GroupConversation: slack.GroupConversation{Name: "private-team", Conversation: slack.Conversation{ID: "C67890"}}},
			},
			privateChannelsError: errors.New("should not be raised"),
			expectedChannelID:    "C12345",
		},
		{
			name:            "channel not found in any accessible channels",
			channelName:     "nonexistent",
			publicChannels:  []slack.Channel{},
			privateChannels: []slack.Channel{},
			expectedError:   "channel not found (check channel name)",
		},
		{
			name:                 "fails when no permissions for either public or private channels",
			channelName:          "any-channel",
			publicChannelsError:  errors.New("missing_scope: channels:read"),
			privateChannelsError: errors.New("missing_scope: groups:read"),
			expectedError:        "missing_scope: channels:read, missing_scope: groups:read (unable to fetch channels, check token and permissions or use channel ID input instead)",
		},
		{
			name:                 "succeeds when public fails but private succeeds",
			channelName:          "private-team",
			publicChannelsError:  errors.New("missing_scope: channels:read"),
			privateChannelsError: nil,
			privateChannels: []slack.Channel{
				{GroupConversation: slack.GroupConversation{Name: "private-team", Conversation: slack.Conversation{ID: "C67890"}}},
			},
			expectedChannelID: "C67890",
			expectedError:     "",
		},
		{
			name:        "fails when public succeeds but private fails and channel not found",
			channelName: "private-only",
			publicChannels: []slack.Channel{
				{GroupConversation: slack.GroupConversation{Name: "other-channel", Conversation: slack.Conversation{ID: "C11111"}}},
			},
			privateChannelsError: errors.New("missing_scope: groups:read"),
			expectedError:        "missing_scope: groups:read (unable to fetch private channels, channel not found from public channels, check channel name, token and permissions or use channel ID input instead)",
		},
		{
			name:                "fails when private succeeds but public fails and channel not found",
			channelName:         "public-only",
			publicChannelsError: errors.New("missing_scope: channels:read"),
			privateChannels: []slack.Channel{
				{GroupConversation: slack.GroupConversation{Name: "other-channel", Conversation: slack.Conversation{ID: "C11111"}}},
			},
			expectedError: "missing_scope: channels:read (unable to fetch public channels, channel not found from private channels, check channel name, token and permissions or use channel ID input instead)",
		},
		{
			name:        "finds channel on a later page of public channels",
			channelName: "general",
			publicChannels: []slack.Channel{
				{GroupConversation: slack.GroupConversation{Name: "other-public", Conversation: slack.Conversation{ID: "C11111"}}},
			},
			publicChannelsNextPage: []slack.Channel{
				{GroupConversation: slack.GroupConversation{Name: "general", Conversation: slack.Conversation{ID: "C12345"}}},
			},
			expectedChannelID: "C12345",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPI := &mockSlackAPI{
				publicChannels:         tt.publicChannels,
				publicChannelsNextPage: tt.publicChannelsNextPage,
				privateChannels:        tt.privateChannels,
				publicChannelsError:    tt.publicChannelsError,
				privateChannelsError:   tt.privateChannelsError,
			}
			client := newClientSkippingRetryWaits(mockAPI)

			channelID, err := client.GetChannelIDByName(tt.channelName)

			if tt.expectedError != "" {
				if err == nil {
					t.Errorf("Expected error containing '%s', got no error", tt.expectedError)
					return
				}
				if err.Error() != tt.expectedError {
					t.Errorf("Expected error '%s', got '%s'", tt.expectedError, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("Expected no error, got: %v", err)
				return
			}

			if channelID != tt.expectedChannelID {
				t.Errorf("Expected channel ID '%s', got '%s'", tt.expectedChannelID, channelID)
			}
		})
	}
}

const publicChannelsNextPageCursor = "next-page"

type mockSlackAPI struct {
	publicChannels         []slack.Channel
	publicChannelsNextPage []slack.Channel
	privateChannels        []slack.Channel
	publicChannelsError    error
	privateChannelsError   error
	postMessageErrors      []error
	updateMessageErrors    []error
	deleteMessageErrors    []error
	editCanvasErrors       []error
	postMessageCalls       int
	updateMessageCalls     int
	deleteMessageCalls     int
	editCanvasParams       []slack.EditCanvasParams
	attemptDeadlines       []time.Time
}

func errorOfAttemptRepeatingTheLast(errs []error, callIndex int) error {
	if len(errs) == 0 {
		return nil
	}
	return errs[min(callIndex, len(errs)-1)]
}

func (m *mockSlackAPI) recordAttemptDeadline(ctx context.Context) {
	deadline, _ := ctx.Deadline()
	m.attemptDeadlines = append(m.attemptDeadlines, deadline)
}

func (m *mockSlackAPI) GetConversations(params *slack.GetConversationsParameters) ([]slack.Channel, string, error) {
	if len(params.Types) == 1 {
		switch params.Types[0] {
		case "public_channel":
			if m.publicChannelsError != nil {
				return nil, "", m.publicChannelsError
			}
			if params.Cursor == publicChannelsNextPageCursor {
				return m.publicChannelsNextPage, "", nil
			}
			if m.publicChannelsNextPage != nil {
				return m.publicChannels, publicChannelsNextPageCursor, nil
			}
			return m.publicChannels, "", nil
		case "private_channel":
			if m.privateChannelsError != nil {
				return nil, "", m.privateChannelsError
			}
			return m.privateChannels, "", nil
		}
	}

	return nil, "", errors.New("unexpected channel types requested")
}

func (m *mockSlackAPI) PostMessageContext(
	ctx context.Context, channelID string, _ ...slack.MsgOption,
) (string, string, error) {
	m.recordAttemptDeadline(ctx)
	err := errorOfAttemptRepeatingTheLast(m.postMessageErrors, m.postMessageCalls)
	m.postMessageCalls++
	if err != nil {
		return "", "", err
	}
	return channelID, fmt.Sprintf("timestamp-of-attempt-%d", m.postMessageCalls), nil
}

func (m *mockSlackAPI) UpdateMessageContext(
	ctx context.Context, channelID string, timestamp string, _ ...slack.MsgOption,
) (string, string, string, error) {
	m.recordAttemptDeadline(ctx)
	err := errorOfAttemptRepeatingTheLast(m.updateMessageErrors, m.updateMessageCalls)
	m.updateMessageCalls++
	if err != nil {
		return "", "", "", err
	}
	return channelID, timestamp, "updated_timestamp", nil
}

func (m *mockSlackAPI) EditCanvasContext(ctx context.Context, params slack.EditCanvasParams) error {
	m.recordAttemptDeadline(ctx)
	err := errorOfAttemptRepeatingTheLast(m.editCanvasErrors, len(m.editCanvasParams))
	m.editCanvasParams = append(m.editCanvasParams, params)
	return err
}

func (m *mockSlackAPI) DeleteMessageContext(
	ctx context.Context, channelID string, timestamp string,
) (string, string, error) {
	m.recordAttemptDeadline(ctx)
	err := errorOfAttemptRepeatingTheLast(m.deleteMessageErrors, m.deleteMessageCalls)
	m.deleteMessageCalls++
	if err != nil {
		return "", "", err
	}
	return channelID, timestamp, nil
}

func newClientSkippingRetryWaits(slackAPI slackclient.SlackAPI) slackclient.Client {
	retryPolicy, _ := retryhelpers.SkipAndRecordWaits()
	return slackclient.NewClient(slackAPI, retryPolicy)
}

func TestSendMessage(t *testing.T) {
	tests := []struct {
		name             string
		channelID        string
		summaryText      string
		blocksCount      int
		postMessageError error
		expectedError    string
	}{
		{
			name:        "successful message send",
			channelID:   "C12345",
			summaryText: "Test summary",
			blocksCount: 5,
		},
		{
			name:          "message with too many blocks (error)",
			channelID:     "C12345",
			summaryText:   "Test summary",
			blocksCount:   55,
			expectedError: "message has too many blocks for Slack API (limit: 50, was: 55)",
		},
		{
			name:             "Slack API rejects the message",
			channelID:        "C12345",
			summaryText:      "Test summary",
			blocksCount:      5,
			postMessageError: errors.New("channel_not_found"),
			expectedError:    "failed to send Slack message: channel_not_found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPI := &mockSlackAPI{postMessageErrors: []error{tt.postMessageError}}
			client := newClientSkippingRetryWaits(mockAPI)

			blocks := make([]slack.Block, tt.blocksCount)
			message := slack.NewBlockMessage(blocks...)

			_, err := client.SendMessage(tt.channelID, message, tt.summaryText)

			if tt.expectedError != "" {
				if err == nil {
					t.Fatalf("Expected error %q, got nil", tt.expectedError)
				}
				if !strings.Contains(err.Error(), tt.expectedError) {
					t.Fatalf("Expected error to contain %q, got %q", tt.expectedError, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
			}
		})
	}
}

func TestUpdateMessage(t *testing.T) {
	tests := []struct {
		name          string
		channelID     string
		messageTS     string
		summaryText   string
		expectedError string
	}{
		{
			name:        "successful message update",
			channelID:   "C12345",
			messageTS:   "1234567890.123456",
			summaryText: "Updated summary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPI := &mockSlackAPI{}
			client := newClientSkippingRetryWaits(mockAPI)

			blocks := slack.NewBlockMessage()

			_, err := client.UpdateMessage(tt.channelID, tt.messageTS, blocks, tt.summaryText)

			if tt.expectedError != "" {
				if err == nil {
					t.Fatalf("Expected error %q, got nil", tt.expectedError)
				}
				if !strings.Contains(err.Error(), tt.expectedError) {
					t.Fatalf("Expected error to contain %q, got %q", tt.expectedError, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
			}
		})
	}
}

// See slackclient.spec.md § Oddities.
func TestSentMessageInfoRecordsTheBlockArrayAsSent(t *testing.T) {
	message := slack.NewBlockMessage(
		slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn", "*Open PRs*", false, false), nil, nil),
		slack.NewDividerBlock(),
	)
	const expectedBlocks = `[{"type":"section","text":{"type":"mrkdwn","text":"*Open PRs*"}},{"type":"divider"}]`

	client := newClientSkippingRetryWaits(&mockSlackAPI{})

	sentInfo, err := client.SendMessage("C12345", message, "summary")
	if err != nil {
		t.Fatalf("SendMessage: expected no error, got %v", err)
	}
	if string(sentInfo.BlocksAsSent) != expectedBlocks {
		t.Errorf("SendMessage: expected blocks %s, got %s", expectedBlocks, sentInfo.BlocksAsSent)
	}

	updatedInfo, err := client.UpdateMessage("C12345", "1234567890.123456", message, "summary")
	if err != nil {
		t.Fatalf("UpdateMessage: expected no error, got %v", err)
	}
	if string(updatedInfo.BlocksAsSent) != expectedBlocks {
		t.Errorf("UpdateMessage: expected blocks %s, got %s", expectedBlocks, updatedInfo.BlocksAsSent)
	}
}

func TestUpdateMessageMarksANotEditableMessage(t *testing.T) {
	client := newClientSkippingRetryWaits(&mockSlackAPI{
		updateMessageErrors: []error{slack.SlackErrorResponse{Err: "edit_window_closed"}},
	})

	_, err := client.UpdateMessage("C12345", "1234567890.123456", slack.NewBlockMessage(), "summary")

	if !errors.Is(err, slackclient.ErrMessageNotEditable) {
		t.Errorf("Expected ErrMessageNotEditable, got %v", err)
	}
}

func TestDeleteMessage(t *testing.T) {
	tests := []struct {
		name               string
		channelID          string
		messageTS          string
		deleteMessageError error
		expectedError      string
	}{
		{
			name:      "successful message delete",
			channelID: "C12345",
			messageTS: "1234567890.123456",
		},
		{
			name:               "delete message fails with error",
			channelID:          "C12345",
			messageTS:          "1234567890.123456",
			deleteMessageError: errors.New("invalid_auth"),
			expectedError:      "failed to delete Slack message",
		},
		{
			name:               "message_not_found error is handled gracefully",
			channelID:          "C12345",
			messageTS:          "1234567890.123456",
			deleteMessageError: errors.New("message_not_found"),
			expectedError:      "", // Should not return error for message_not_found
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPI := &mockSlackAPI{
				deleteMessageErrors: []error{tt.deleteMessageError},
			}
			client := newClientSkippingRetryWaits(mockAPI)

			err := client.DeleteMessage(tt.channelID, tt.messageTS)

			if tt.expectedError != "" {
				if err == nil {
					t.Fatalf("Expected error %q, got nil", tt.expectedError)
				}
				if !strings.Contains(err.Error(), tt.expectedError) {
					t.Fatalf("Expected error to contain %q, got %q", tt.expectedError, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
			}
		})
	}
}

func TestReplaceCanvasContent(t *testing.T) {
	tests := []struct {
		name            string
		canvasID        string
		markdown        string
		editCanvasError error
		expectedError   string
	}{
		{
			name:     "successful canvas content replace",
			canvasID: "F0BMEPVR1DL",
			markdown: "## Open PRs\n\n- one",
		},
		{
			name:            "canvas edit failure is wrapped with a permission hint",
			canvasID:        "F0BMEPVR1DL",
			markdown:        "## Open PRs",
			editCanvasError: errors.New("canvas_not_found"),
			expectedError: "canvas update failed: check that the bot has canvases:write permission " +
				"and is invited to the channel where the canvas is: canvas_not_found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPI := &mockSlackAPI{editCanvasErrors: []error{tt.editCanvasError}}
			client := newClientSkippingRetryWaits(mockAPI)

			err := client.ReplaceCanvasContent(tt.canvasID, tt.markdown)

			if tt.expectedError != "" {
				if err == nil {
					t.Fatalf("Expected error %q, got nil", tt.expectedError)
				}
				if err.Error() != tt.expectedError {
					t.Fatalf("Expected error %q, got %q", tt.expectedError, err.Error())
				}
			} else if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}

			if len(mockAPI.editCanvasParams) != 1 {
				t.Fatalf("Expected exactly one EditCanvas call, got %d", len(mockAPI.editCanvasParams))
			}
			params := mockAPI.editCanvasParams[0]
			if params.CanvasID != tt.canvasID {
				t.Errorf("Expected canvas ID %q, got %q", tt.canvasID, params.CanvasID)
			}
			if len(params.Changes) != 1 {
				t.Fatalf("Expected exactly one change, got %d", len(params.Changes))
			}
			change := params.Changes[0]
			if change.Operation != "replace" {
				t.Errorf("Expected operation \"replace\", got %q", change.Operation)
			}
			if change.SectionID != "" {
				t.Errorf("Expected empty section ID, got %q", change.SectionID)
			}
			if change.DocumentContent.Type != "markdown" {
				t.Errorf("Expected document content type \"markdown\", got %q", change.DocumentContent.Type)
			}
			if change.DocumentContent.Markdown != tt.markdown {
				t.Errorf("Expected markdown %q, got %q", tt.markdown, change.DocumentContent.Markdown)
			}
		})
	}
}

type slackCallUnderTest struct {
	apiName          string
	scriptErrors     func(m *mockSlackAPI, errs []error)
	call             func(client slackclient.Client) error
	attempts         func(m *mockSlackAPI) int
	transientErrors  []error
	permanentError   string
	lastErrorAfter3  string
	transientLogLine string
}

var slackCallsUnderTest = []slackCallUnderTest{
	{
		apiName:      "Slack post",
		scriptErrors: func(m *mockSlackAPI, errs []error) { m.postMessageErrors = errs },
		call: func(client slackclient.Client) error {
			_, err := client.SendMessage("C12345", slack.NewBlockMessage(), "summary")
			return err
		},
		attempts: func(m *mockSlackAPI) int { return m.postMessageCalls },
		transientErrors: []error{
			slack.StatusCodeError{Code: 429, Status: "429 Too Many Requests"},
			slack.SlackErrorResponse{Err: "ratelimited"},
			netOpError("dial"),
		},
		permanentError:   "failed to send Slack message: invalid_auth",
		lastErrorAfter3:  `failed to send Slack message: Post "https://slack.com/api/chat.postMessage": dial tcp: connection refused`,
		transientLogLine: "Slack post attempt 1 failed, retrying in 2s: slack server error: 429 Too Many Requests",
	},
	{
		apiName:      "Slack update",
		scriptErrors: func(m *mockSlackAPI, errs []error) { m.updateMessageErrors = errs },
		call: func(client slackclient.Client) error {
			_, err := client.UpdateMessage("C12345", "1234567890.123456", slack.NewBlockMessage(), "summary")
			return err
		},
		attempts:         func(m *mockSlackAPI) int { return m.updateMessageCalls },
		transientErrors:  idempotentCallTransientErrors,
		permanentError:   "failed to update Slack message: invalid_auth",
		lastErrorAfter3:  "failed to update Slack message: fatal_error",
		transientLogLine: "Slack update attempt 1 failed, retrying in 2s: slack server error: 502 Bad Gateway",
	},
	{
		apiName:      "Slack delete",
		scriptErrors: func(m *mockSlackAPI, errs []error) { m.deleteMessageErrors = errs },
		call: func(client slackclient.Client) error {
			return client.DeleteMessage("C12345", "1234567890.123456")
		},
		attempts:         func(m *mockSlackAPI) int { return m.deleteMessageCalls },
		transientErrors:  idempotentCallTransientErrors,
		permanentError:   "failed to delete Slack message: invalid_auth",
		lastErrorAfter3:  "failed to delete Slack message: fatal_error",
		transientLogLine: "Slack delete attempt 1 failed, retrying in 2s: slack server error: 502 Bad Gateway",
	},
	{
		apiName:      "Slack canvas edit",
		scriptErrors: func(m *mockSlackAPI, errs []error) { m.editCanvasErrors = errs },
		call: func(client slackclient.Client) error {
			return client.ReplaceCanvasContent("F0BMEPVR1DL", "## Open PRs")
		},
		attempts:        func(m *mockSlackAPI) int { return len(m.editCanvasParams) },
		transientErrors: idempotentCallTransientErrors,
		permanentError: "canvas update failed: check that the bot has canvases:write permission " +
			"and is invited to the channel where the canvas is: invalid_auth",
		lastErrorAfter3: "canvas update failed: check that the bot has canvases:write permission " +
			"and is invited to the channel where the canvas is: fatal_error",
		transientLogLine: "Slack canvas edit attempt 1 failed, retrying in 2s: slack server error: 502 Bad Gateway",
	},
}

// Each is transient for an idempotent call only, so a call wired to the post rule fails on them.
var idempotentCallTransientErrors = []error{
	slack.StatusCodeError{Code: 502, Status: "502 Bad Gateway"},
	slack.SlackErrorResponse{Err: "internal_error"},
	slack.SlackErrorResponse{Err: "fatal_error"},
}

func TestSlackCallsRetryTransientFailures(t *testing.T) {
	for _, slackCall := range slackCallsUnderTest {
		scenarios := []struct {
			name             string
			errs             []error
			expectedAttempts int
			expectedError    string
			expectedLogLines []string
		}{
			{
				name:             "transient failure then success",
				errs:             []error{slackCall.transientErrors[0], nil},
				expectedAttempts: 2,
				expectedLogLines: []string{slackCall.transientLogLine},
			},
			{
				name:             "permanent failure",
				errs:             []error{slack.SlackErrorResponse{Err: "invalid_auth"}, nil},
				expectedAttempts: 1,
				expectedError:    slackCall.permanentError,
			},
			{
				name:             "transient failure on every attempt",
				errs:             append(slices.Clone(slackCall.transientErrors), nil),
				expectedAttempts: 3,
				expectedError:    slackCall.lastErrorAfter3,
			},
		}

		for _, scenario := range scenarios {
			t.Run(slackCall.apiName+": "+scenario.name, func(t *testing.T) {
				logOutput := testhelpers.CaptureLog(t)
				mockAPI := &mockSlackAPI{}
				slackCall.scriptErrors(mockAPI, scenario.errs)
				startedAt := time.Now()

				err := slackCall.call(newClientSkippingRetryWaits(mockAPI))

				if errorText(err) != scenario.expectedError {
					t.Errorf("expected error %q, got %q", scenario.expectedError, errorText(err))
				}
				if attempts := slackCall.attempts(mockAPI); attempts != scenario.expectedAttempts {
					t.Errorf("expected %d attempts, got %d", scenario.expectedAttempts, attempts)
				}
				if scenario.expectedLogLines != nil {
					logLines := testhelpers.LogLinesStartingWith(logOutput, slackCall.apiName+" attempt")
					if !slices.Equal(logLines, scenario.expectedLogLines) {
						t.Errorf("expected retry log lines %q, got %q", scenario.expectedLogLines, logLines)
					}
				}
				assertEachAttemptHad15sDeadline(t, mockAPI.attemptDeadlines, startedAt)
			})
		}
	}
}

func assertEachAttemptHad15sDeadline(t *testing.T, deadlines []time.Time, startedAt time.Time) {
	t.Helper()
	finishedAt := time.Now()
	for index, deadline := range deadlines {
		if deadline.Before(startedAt.Add(15*time.Second)) || deadline.After(finishedAt.Add(15*time.Second)) {
			t.Errorf("attempt %d: expected a deadline 15s after it started, got %v", index+1, deadline)
		}
	}
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestSendMessageReturnsTheSucceedingAttemptsMessage(t *testing.T) {
	mockAPI := &mockSlackAPI{postMessageErrors: []error{slack.SlackErrorResponse{Err: "ratelimited"}, nil}}

	sentInfo, err := newClientSkippingRetryWaits(mockAPI).SendMessage("C12345", slack.NewBlockMessage(), "summary")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if sentInfo.Timestamp != "timestamp-of-attempt-2" {
		t.Errorf("expected the second attempt's timestamp, got %q", sentInfo.Timestamp)
	}
}

func netOpError(op string) error {
	return &url.Error{
		Op:  "Post",
		URL: "https://slack.com/api/chat.postMessage",
		Err: &net.OpError{Op: op, Net: "tcp", Err: errors.New("connection refused")},
	}
}

// See slackclient.spec.md § Behaviour.
func TestSendMessageRetriesOnlyFailuresWhereSlackDidNotPost(t *testing.T) {
	tests := []struct {
		name            string
		err             error
		expectedRetried bool
	}{
		{name: "rate limited with Retry-After", err: &slack.RateLimitedError{RetryAfter: 30 * time.Second}, expectedRetried: true},
		{name: "status 429", err: slack.StatusCodeError{Code: 429, Status: "429 Too Many Requests"}, expectedRetried: true},
		{name: "ratelimited code", err: slack.SlackErrorResponse{Err: "ratelimited"}, expectedRetried: true},
		{name: "dial failure", err: netOpError("dial"), expectedRetried: true},
		{name: "service_unavailable code", err: slack.SlackErrorResponse{Err: "service_unavailable"}},
		{name: "status 502", err: slack.StatusCodeError{Code: 502, Status: "502 Bad Gateway"}},
		{name: "internal_error code", err: slack.SlackErrorResponse{Err: "internal_error"}},
		{name: "fatal_error code", err: slack.SlackErrorResponse{Err: "fatal_error"}},
		{name: "attempt deadline", err: context.DeadlineExceeded},
		{name: "read failure after sending", err: netOpError("read")},
		{name: "channel_not_found code", err: slack.SlackErrorResponse{Err: "channel_not_found"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPI := &mockSlackAPI{postMessageErrors: []error{tt.err, nil}}

			_, _ = newClientSkippingRetryWaits(mockAPI).SendMessage("C12345", slack.NewBlockMessage(), "summary")

			if retried := mockAPI.postMessageCalls == 2; retried != tt.expectedRetried {
				t.Errorf("expected retried %v, got %d attempts", tt.expectedRetried, mockAPI.postMessageCalls)
			}
		})
	}
}

func TestUpdateMessageRetriesIdempotentCallTransientFailures(t *testing.T) {
	tests := []struct {
		name            string
		err             error
		expectedRetried bool
	}{
		{name: "rate limited with Retry-After", err: &slack.RateLimitedError{RetryAfter: 30 * time.Second}, expectedRetried: true},
		{name: "status 429", err: slack.StatusCodeError{Code: 429, Status: "429 Too Many Requests"}, expectedRetried: true},
		{name: "status 500", err: slack.StatusCodeError{Code: 500, Status: "500 Internal Server Error"}, expectedRetried: true},
		{name: "ratelimited code", err: slack.SlackErrorResponse{Err: "ratelimited"}, expectedRetried: true},
		{name: "service_unavailable code", err: slack.SlackErrorResponse{Err: "service_unavailable"}, expectedRetried: true},
		{name: "request_timeout code", err: slack.SlackErrorResponse{Err: "request_timeout"}, expectedRetried: true},
		{name: "internal_error code", err: slack.SlackErrorResponse{Err: "internal_error"}, expectedRetried: true},
		{name: "fatal_error code", err: slack.SlackErrorResponse{Err: "fatal_error"}, expectedRetried: true},
		{name: "dial failure", err: netOpError("dial"), expectedRetried: true},
		{name: "read failure after sending", err: netOpError("read"), expectedRetried: true},
		{name: "attempt deadline", err: context.DeadlineExceeded, expectedRetried: true},
		{name: "status 404", err: slack.StatusCodeError{Code: 404, Status: "404 Not Found"}},
		{name: "invalid_auth code", err: slack.SlackErrorResponse{Err: "invalid_auth"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPI := &mockSlackAPI{updateMessageErrors: []error{tt.err, nil}}

			_, _ = newClientSkippingRetryWaits(mockAPI).UpdateMessage("C12345", "1234567890.123456", slack.NewBlockMessage(), "summary")

			if retried := mockAPI.updateMessageCalls == 2; retried != tt.expectedRetried {
				t.Errorf("expected retried %v, got %d attempts", tt.expectedRetried, mockAPI.updateMessageCalls)
			}
		})
	}
}

func TestUpdateMessageDoesNotRetryANotEditableMessage(t *testing.T) {
	mockAPI := &mockSlackAPI{updateMessageErrors: []error{slack.SlackErrorResponse{Err: "message_not_found"}, nil}}

	_, err := newClientSkippingRetryWaits(mockAPI).UpdateMessage("C12345", "1234567890.123456", slack.NewBlockMessage(), "summary")

	if !errors.Is(err, slackclient.ErrMessageNotEditable) {
		t.Errorf("expected ErrMessageNotEditable, got %v", err)
	}
	if mockAPI.updateMessageCalls != 1 {
		t.Errorf("expected 1 attempt, got %d", mockAPI.updateMessageCalls)
	}
}

// See slackclient.spec.md § Oddities.
func TestDeleteMessageTreatsMessageNotFoundAfterATransientFailureAsDeleted(t *testing.T) {
	mockAPI := &mockSlackAPI{deleteMessageErrors: []error{
		slack.StatusCodeError{Code: 502, Status: "502 Bad Gateway"},
		slack.SlackErrorResponse{Err: "message_not_found"},
	}}

	err := newClientSkippingRetryWaits(mockAPI).DeleteMessage("C12345", "1234567890.123456")

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if mockAPI.deleteMessageCalls != 2 {
		t.Errorf("expected 2 attempts, got %d", mockAPI.deleteMessageCalls)
	}
}
