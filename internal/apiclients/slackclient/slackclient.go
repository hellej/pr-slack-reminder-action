package slackclient

import (
	"errors"
	"fmt"
	"log"

	"github.com/slack-go/slack"
)

var ErrChannelNotFound = errors.New("channel not found")

func GetChannelIDByName(api *slack.Client, channelName string) (string, error) {
	channels, cursor := []slack.Channel{}, ""

	for {
		result, nextCursor, err := api.GetConversations(&slack.GetConversationsParameters{
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
	return "", ErrChannelNotFound
}

func GetClient(token string) *slack.Client {
	return slack.New(token)
}

func SendMessage(api *slack.Client, channelID string, blocks slack.Message, summaryText string) error {
	_, _, err := api.PostMessage(
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
