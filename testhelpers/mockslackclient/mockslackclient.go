package mockslackclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/slackclient"
	"github.com/slack-go/slack"
)

type MockSlackClientOptions struct {
	SlackChannels      []*SlackChannel
	FindChannelError   error
	PostMessageError   error
	UpdateMessageError error
	DeleteMessageError error
}

func MakeSlackClientGetter(slackAPI *MockSlackAPI) func(token string) slackclient.Client {
	return func(token string) slackclient.Client {
		return slackAPI
	}
}

func GetMockSlackAPI(opts MockSlackClientOptions) *MockSlackAPI {
	if opts.SlackChannels == nil {
		opts.SlackChannels = []*SlackChannel{
			{ID: "C12345678", Name: "some-channel-name"},
		}
	}
	channels := make([]slack.Channel, len(opts.SlackChannels))
	for i, channel := range opts.SlackChannels {
		channels[i] = slack.Channel{
			GroupConversation: slack.GroupConversation{
				Name: channel.Name,
				Conversation: slack.Conversation{
					ID: channel.ID,
				},
			},
		}
	}
	return &MockSlackAPI{
		slackChannels:      channels,
		findChannelError:   opts.FindChannelError,
		postMessageError:   opts.PostMessageError,
		updateMessageError: opts.UpdateMessageError,
		deleteMessageError: opts.DeleteMessageError,
		postMessageResponse: PostMessageResponse{
			Timestamp: "1234567890.123456",
			Channel:   "C12345678",
		},
	}
}

type MockSlackAPI struct {
	slackChannels       []slack.Channel
	findChannelError    error
	postMessageError    error
	updateMessageError  error
	deleteMessageError  error
	postMessageResponse PostMessageResponse
	SentMessage         SentMessage
	UpdatedMessage      UpdatedMessage
	DeletedMessage      DeletedMessage
}

func (m *MockSlackAPI) GetChannelIDByName(channelName string) (string, error) {
	if m.findChannelError != nil {
		return "", fmt.Errorf(
			"%v, %v (unable to fetch channels, check token and permissions or use channel ID input instead)",
			m.findChannelError,
			m.findChannelError,
		)
	}

	for _, channel := range m.slackChannels {
		if channel.Name == channelName {
			return channel.ID, nil
		}
	}

	return "", errors.New("channel not found (check channel name)")
}

func (m *MockSlackAPI) SendMessage(
	channelID string,
	message slack.Message,
	summaryText string,
) (slackclient.SentMessageInfo, error) {
	sentBlocks, err := parseBlocksFromMessage(message)
	if err != nil {
		panic("Failed to parse sent blocks in mock Slack API: " + err.Error())
	}

	if m.postMessageError == nil {
		m.SentMessage.ChannelID = channelID
		m.SentMessage.Text = summaryText
		m.SentMessage.Blocks = sentBlocks
	}

	if m.postMessageError != nil {
		return slackclient.SentMessageInfo{}, fmt.Errorf("failed to send Slack message: %v", m.postMessageError)
	}

	jsonBlocks := getJSONBlocks(message)
	return slackclient.SentMessageInfo{
		ChannelID:  m.postMessageResponse.Channel,
		Timestamp:  m.postMessageResponse.Timestamp,
		JSONBlocks: jsonBlocks,
	}, nil
}

func (m *MockSlackAPI) UpdateMessage(
	channelID string,
	messageTS string,
	message slack.Message,
	summaryText string,
) (slackclient.SentMessageInfo, error) {
	updatedBlocks, err := parseBlocksFromMessage(message)
	if err != nil {
		panic("Failed to parse updated blocks in mock Slack API: " + err.Error())
	}

	if m.updateMessageError == nil {
		m.UpdatedMessage.ChannelID = channelID
		m.UpdatedMessage.Timestamp = messageTS
		m.UpdatedMessage.Text = summaryText
		m.UpdatedMessage.Blocks = updatedBlocks
	}

	if m.updateMessageError != nil {
		return slackclient.SentMessageInfo{}, fmt.Errorf("failed to update Slack message: %v", m.updateMessageError)
	}

	jsonBlocks := getJSONBlocks(message)
	return slackclient.SentMessageInfo{
		ChannelID:  channelID,
		Timestamp:  messageTS,
		JSONBlocks: jsonBlocks,
	}, nil
}

func (m *MockSlackAPI) DeleteMessage(channelID string, timestamp string) error {
	// Always record the delete attempt, even if it fails
	m.DeletedMessage.ChannelID = channelID
	m.DeletedMessage.Timestamp = timestamp
	if m.deleteMessageError != nil {
		if strings.Contains(m.deleteMessageError.Error(), "message_not_found") {
			return nil
		}
		return fmt.Errorf("failed to delete Slack message: %v", m.deleteMessageError)
	}
	return nil
}

type SlackChannel struct {
	ID   string
	Name string
}

type PostMessageResponse struct {
	Timestamp string
	Channel   string
}

// To allow storing and asserting the request in tests
type SentMessage struct {
	ChannelID string
	Blocks    BlocksWrapper
	Text      string
}

type UpdatedMessage struct {
	ChannelID string
	Timestamp string
	Blocks    BlocksWrapper
	Text      string
}

type DeletedMessage struct {
	ChannelID string
	Timestamp string
}

func parseBlocksFromMessage(message slack.Message) (BlocksWrapper, error) {
	blockBytes, err := json.Marshal(message.Blocks.BlockSet)
	if err != nil {
		return BlocksWrapper{}, err
	}
	if len(blockBytes) == 0 {
		return BlocksWrapper{}, nil
	}
	return ParseBlocks(blockBytes)
}

func getJSONBlocks(message slack.Message) []string {
	blockBytes, err := json.Marshal(message.Blocks.BlockSet)
	if err != nil {
		return []string{}
	}
	return []string{string(blockBytes)}
}
