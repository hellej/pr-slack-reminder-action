package slackclient

import (
	"errors"
	"fmt"
	"log"

	"github.com/slack-go/slack"
)

type Client interface {
	GetChannelIDByName(channelName string) (string, error)
	SendMessage(channelID string, blocks slack.Message, summaryText string) error
}

type slackAPI interface {
	GetConversations(params *slack.GetConversationsParameters) ([]slack.Channel, string, error)
	PostMessage(channelID string, options ...slack.MsgOption) (string, string, error)
}

type client struct {
	slackAPI slackAPI
}

func NewClient(slackAPI slackAPI) Client {
	return &client{slackAPI: slackAPI}
}

func GetAuthenticatedClient(token string) Client {
	return NewClient(slack.New(token))
}

func (c *client) GetChannelIDByName(channelName string) (string, error) {
	channels, cursor := []slack.Channel{}, ""

	for {
		result, nextCursor, err := c.slackAPI.GetConversations(&slack.GetConversationsParameters{
			Limit:           200,
			Cursor:          cursor,
			Types:           []string{"public_channel", "private_channel"},
			ExcludeArchived: true,
		})
		if err != nil {
			return "", err
		}
		channels = append(channels, result...)
		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	for _, ch := range channels {
		if ch.Name == channelName {
			return ch.ID, nil
		}
	}
	return "", errors.New("channel not found")
}

func (c client) SendMessage(channelID string, blocks slack.Message, summaryText string) error {
	_, _, err := c.slackAPI.PostMessage(
		channelID,
		slack.MsgOptionBlocks(blocks.Blocks.BlockSet...),
		slack.MsgOptionText(summaryText, false),
	)
	if err != nil {
		return fmt.Errorf("failed to send Slack message: %v", err)
	}
	log.Printf("Sent message to Slack channel: %s", channelID)
	return nil
}
