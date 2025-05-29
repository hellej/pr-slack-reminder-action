package slackhelpers

import (
	"errors"
	"fmt"
	"log"

	"github.com/slack-go/slack"
)

var ErrChannelNotFound = errors.New("channel not found")

func getChannelIDByName(api *slack.Client, channelName string) (string, error) {
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

func SendMessage(api *slack.Client, channelName string, blocks slack.Message, summaryText string) error {
	channelID, err := getChannelIDByName(api, channelName)
	if err != nil {
		log.Fatalf("Error getting channel ID by name: %v (%v)", channelName, err)
	}

	log.Printf("Sending message to channel \"%s\"", channelName)
	_, _, err = api.PostMessage(channelID, slack.MsgOptionBlocks(blocks.Blocks.BlockSet...), slack.MsgOptionText(summaryText, false))

	if err != nil {
		return fmt.Errorf("failed to send Slack message: %s", err.Error())
	}
	return nil
}
