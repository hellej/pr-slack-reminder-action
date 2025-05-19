package slacknotifier

import (
	"errors"
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

func SendMessage(api *slack.Client, channelName string, blocks slack.Message) error {
	log.Printf("Finding channel ID by name: %s", channelName)

	channelID, err := getChannelIDByName(api, channelName)
	if err != nil {
		return err
	}

	log.Printf("Sending message to channel (ID): %s", channelID)
	log.Printf("Message: %s", blocks.ClientMsgID)

	_, _, err = api.PostMessage(channelID, slack.MsgOptionBlocks(blocks.Blocks.BlockSet...))
	if err != nil {
		return err
	}

	return nil
}
