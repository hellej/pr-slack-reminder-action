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

type client struct {
	client *slack.Client
}

func GetClient(token string) Client {
	return client{client: slack.New(token)}
}

func (c client) GetChannelIDByName(channelName string) (string, error) {
	channels, cursor := []slack.Channel{}, ""

	for {
		result, nextCursor, err := c.client.GetConversations(&slack.GetConversationsParameters{
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
	_, _, err := c.client.PostMessage(
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
