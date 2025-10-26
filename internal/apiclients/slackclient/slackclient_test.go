package slackclient_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/slack-go/slack"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/slackclient"
)

func TestGetAuthenticatedClient(t *testing.T) {
	client := slackclient.GetAuthenticatedClient("test-token")
	if client == nil {
		t.Fatal("Expected non-nil client, got nil")
	}
}

func TestGetChannelIDByName(t *testing.T) {
	tests := []struct {
		name                 string
		channelName          string
		publicChannels       []slack.Channel
		privateChannels      []slack.Channel
		publicChannelsError  error
		privateChannelsError error
		expectedChannelID    string
		expectedError        string
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPI := &mockSlackAPI{
				publicChannels:       tt.publicChannels,
				privateChannels:      tt.privateChannels,
				publicChannelsError:  tt.publicChannelsError,
				privateChannelsError: tt.privateChannelsError,
			}
			client := slackclient.NewClient(mockAPI)

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

type mockSlackAPI struct {
	publicChannels       []slack.Channel
	privateChannels      []slack.Channel
	publicChannelsError  error
	privateChannelsError error
	callCount            int
}

func (m *mockSlackAPI) GetConversations(params *slack.GetConversationsParameters) ([]slack.Channel, string, error) {
	m.callCount++

	// The new implementation calls GetConversations separately for each channel type
	// Check what type is being requested
	if len(params.Types) == 1 {
		switch params.Types[0] {
		case "public_channel":
			if m.publicChannelsError != nil {
				return nil, "", m.publicChannelsError
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

func (m *mockSlackAPI) PostMessage(channelID string, options ...slack.MsgOption) (string, string, error) {
	return "timestamp", channelID, nil
}

func (m *mockSlackAPI) UpdateMessage(channelID string, timestamp string, options ...slack.MsgOption) (string, string, string, error) {
	return channelID, timestamp, "updated_timestamp", nil
}

func TestSendMessage(t *testing.T) {
	tests := []struct {
		name          string
		channelID     string
		summaryText   string
		expectedError string
	}{
		{
			name:        "successful message send",
			channelID:   "C12345",
			summaryText: "Test summary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPI := &mockSlackAPI{}
			client := slackclient.NewClient(mockAPI)

			blocks := slack.NewBlockMessage()

			_, err := client.SendMessage(tt.channelID, blocks, tt.summaryText)

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
			client := slackclient.NewClient(mockAPI)

			blocks := slack.NewBlockMessage()

			err := client.UpdateMessage(tt.channelID, tt.messageTS, blocks, tt.summaryText)

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
