package mockslackclient

import (
	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/slackclient"
	"github.com/slack-go/slack"
)

// creates the MockSlackAPI (for dependency injection) if nil is provided
func MakeSlackClientGetter(slackAPI *MockSlackAPI) func(token string) slackclient.Client {
	if slackAPI == nil {
		slackAPI = GetMockSlackAPI(nil, nil, nil)
	}
	return func(token string) slackclient.Client {
		return slackclient.NewClient(slackAPI)
	}
}

func GetMockSlackAPI(
	slackChannels []*SlackChannel,
	findChannelError error,
	postMessageError error,
) *MockSlackAPI {
	return GetMockSlackAPIWithUpdateError(slackChannels, findChannelError, postMessageError, nil)
}

func GetMockSlackAPIWithUpdateError(
	slackChannels []*SlackChannel,
	findChannelError error,
	postMessageError error,
	updateMessageError error,
) *MockSlackAPI {
	if slackChannels == nil {
		slackChannels = []*SlackChannel{
			{ID: "C12345678", Name: "some-channel-name"},
		}
	}
	channels := make([]slack.Channel, len(slackChannels))
	for i, channel := range slackChannels {
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
		getConversationsResponse: GetConversationsResponse{
			channels: channels,
			cursor:   "",
			err:      findChannelError,
		},
		postMessageResponse: PostMessageResponse{
			Timestamp: "1234567890.123456",
			Channel:   "C12345678",
			Err:       postMessageError,
		},
		updateMessageResponse: UpdateMessageResponse{
			Channel:   "C12345678",
			Timestamp: "1234567890.123456",
			Text:      "updated text",
			Err:       updateMessageError,
		},
	}
}

type MockSlackAPI struct {
	getConversationsResponse GetConversationsResponse
	postMessageResponse      PostMessageResponse
	updateMessageResponse    UpdateMessageResponse
	SentMessage              SentMessage
	UpdatedMessage           UpdatedMessage
}

func (m *MockSlackAPI) GetConversations(params *slack.GetConversationsParameters) ([]slack.Channel, string, error) {
	if m.getConversationsResponse.err != nil {
		return nil, "", m.getConversationsResponse.err
	}
	return m.getConversationsResponse.channels, m.getConversationsResponse.cursor, nil
}

func (m *MockSlackAPI) PostMessage(
	channelID string, options ...slack.MsgOption,
) (string, string, error) {
	request, values, err := slack.UnsafeApplyMsgOptions("", "", "", options...)

	if err != nil {
		panic("Failed to apply message options in mock Slack API: " + err.Error())
	}

	var sentBlocks BlocksWrapper
	if blocks, ok := values["blocks"]; ok && len(blocks) > 0 {
		sentBlocks, err = ParseBlocks([]byte(blocks[0]))
	}

	if err != nil {
		panic("Failed to parse sent blocks in mock Slack API: " + err.Error())
	}

	if m.postMessageResponse.Err == nil {
		m.SentMessage.Request = request
		m.SentMessage.ChannelID = channelID
		m.SentMessage.Text = values["text"][0]
		m.SentMessage.Blocks = sentBlocks
	}
	return m.postMessageResponse.Channel, m.postMessageResponse.Timestamp, m.postMessageResponse.Err
}

func (m *MockSlackAPI) UpdateMessage(
	channelID string, timestamp string, options ...slack.MsgOption,
) (string, string, string, error) {
	_, values, err := slack.UnsafeApplyMsgOptions("", "", "", options...)

	if err != nil {
		panic("Failed to apply message options in mock Slack API: " + err.Error())
	}

	var updatedBlocks BlocksWrapper
	if blocks, ok := values["blocks"]; ok && len(blocks) > 0 {
		updatedBlocks, err = ParseBlocks([]byte(blocks[0]))
	}

	if err != nil {
		panic("Failed to parse updated blocks in mock Slack API: " + err.Error())
	}

	if m.updateMessageResponse.Err == nil {
		m.UpdatedMessage.ChannelID = channelID
		m.UpdatedMessage.Timestamp = timestamp
		m.UpdatedMessage.Text = values["text"][0]
		m.UpdatedMessage.Blocks = updatedBlocks
	}
	return channelID, timestamp, "updated_timestamp", m.updateMessageResponse.Err
}

type SlackChannel struct {
	ID   string
	Name string
}

type GetConversationsResponse struct {
	channels []slack.Channel
	cursor   string
	err      error
}

type PostMessageResponse struct {
	Timestamp string
	Channel   string
	Err       error
}

type UpdateMessageResponse struct {
	Channel   string
	Timestamp string
	Text      string
	Err       error
}

// To allow storing and asserting the request in tests
type SentMessage struct {
	Request   string
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

// GetLastUpdateMessage returns the details of the last message update call
func (m *MockSlackAPI) GetLastUpdateMessage() UpdatedMessage {
	return m.UpdatedMessage
}
