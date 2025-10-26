// Package slackclient provides Slack API integration for sending messages.
// It handles channel resolution by name and message posting with Block Kit formatting.
package slackclient

import (
	"errors"
	"fmt"
	"log"

	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
	"github.com/slack-go/slack"
)

type MessageResponse struct {
	ChannelID string
	Timestamp string
}

type Client interface {
	GetChannelIDByName(channelName string) (string, error)
	SendMessage(channelID string, blocks slack.Message, summaryText string) (*MessageResponse, error)
	UpdateMessage(channelID string, messageTS string, blocks slack.Message, summaryText string) error
}

func GetAuthenticatedClient(token string) Client {
	return NewClient(slack.New(token))
}

func NewClient(slackAPI SlackAPI) Client {
	return &client{slackAPI: slackAPI}
}

// represents the Slack API methods relevant to us from github.com/slack-go/slack
type SlackAPI interface {
	GetConversations(params *slack.GetConversationsParameters) ([]slack.Channel, string, error)
	PostMessage(channelID string, options ...slack.MsgOption) (string, string, error)
	UpdateMessage(channelID string, timestamp string, options ...slack.MsgOption) (string, string, string, error)
}

type client struct {
	slackAPI SlackAPI
}

func (c *client) GetChannelIDByName(channelName string) (string, error) {
	var publicChannelsError error
	var privateChannelsError error

	for _, channelType := range []string{"public_channel", "private_channel"} {
		channels, fetchError := c.fetchChannels([]string{channelType})
		if fetchError != nil {
			if channelType == "public_channel" {
				publicChannelsError = fetchError
			} else {
				privateChannelsError = fetchError
			}
			continue
		}
		channel, found := utilities.Find(channels, func(ch slack.Channel) bool {
			return ch.Name == channelName
		})
		if found {
			return channel.ID, nil
		}
	}

	if publicChannelsError == nil && privateChannelsError != nil {
		return "", fmt.Errorf(
			"%v (unable to fetch private channels, channel not found from public channels, "+
				"check channel name, token and permissions or use channel ID input instead)",
			privateChannelsError,
		)
	}
	if publicChannelsError != nil && privateChannelsError == nil {
		return "", fmt.Errorf(
			"%v (unable to fetch public channels, channel not found from private channels, "+
				"check channel name, token and permissions or use channel ID input instead)",
			publicChannelsError,
		)
	}
	if publicChannelsError != nil && privateChannelsError != nil {
		return "", fmt.Errorf(
			"%v, %v (unable to fetch channels, check token and permissions or use channel ID input instead)",
			publicChannelsError,
			privateChannelsError,
		)
	}

	return "", errors.New("channel not found (check channel name)")
}

func (c *client) SendMessage(channelID string, blocks slack.Message, summaryText string) (*MessageResponse, error) {
	log.Printf("\nSending message with summary: %s", summaryText)
	responseChannelID, timestamp, err := c.slackAPI.PostMessage(
		channelID,
		slack.MsgOptionBlocks(blocks.Blocks.BlockSet...),
		slack.MsgOptionText(summaryText, false),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to send Slack message: %v", err)
	}
	log.Printf("Sent message to Slack channel: %s", channelID)
	return &MessageResponse{
		ChannelID: responseChannelID,
		Timestamp: timestamp,
	}, nil
}

func (c *client) UpdateMessage(channelID string, messageTS string, blocks slack.Message, summaryText string) error {
	log.Printf("Updating message with timestamp %s and summary: %s", messageTS, summaryText)
	_, _, _, err := c.slackAPI.UpdateMessage(
		channelID,
		messageTS,
		slack.MsgOptionBlocks(blocks.Blocks.BlockSet...),
		slack.MsgOptionText(summaryText, false),
	)
	if err != nil {
		return fmt.Errorf("failed to update Slack message: %v", err)
	}
	log.Printf("Updated message in Slack channel: %s", channelID)
	return nil
}

func (c *client) fetchChannels(types []string) ([]slack.Channel, error) {
	channels, cursor := []slack.Channel{}, ""

	for {
		result, nextCursor, err := c.slackAPI.GetConversations(&slack.GetConversationsParameters{
			Limit:           999,
			Cursor:          cursor,
			Types:           types,
			ExcludeArchived: true,
		})
		if err != nil {
			return nil, err
		}
		channels = append(channels, result...)
		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	return channels, nil
}
