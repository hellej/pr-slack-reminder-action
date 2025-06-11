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
	}
}

type MockSlackAPI struct {
	getConversationsResponse GetConversationsResponse
	postMessageResponse      PostMessageResponse
	SentMessage              SentMessage
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
	request, values, _ := slack.UnsafeApplyMsgOptions("", "", "", options...)
	if m.postMessageResponse.Err == nil {
		m.SentMessage.Request = request
		m.SentMessage.ChannelID = channelID
		m.SentMessage.Text = values["text"][0]
	}
	return "", "", m.postMessageResponse.Err
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

// to allow storing and asserting the request in tests
type SentMessage struct {
	Request   string
	ChannelID string
	Blocks    slack.Message
	Text      string
}
