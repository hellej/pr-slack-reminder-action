// Package slackclient provides Slack API integration for channel resolution by name,
// message posting with Block Kit formatting, and full canvas content replacement by canvas ID.
package slackclient

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
	"github.com/slack-go/slack"
)

type SentMessageInfo struct {
	ChannelID  string
	Timestamp  string
	JSONBlocks []string
}

type Client interface {
	GetChannelIDByName(channelName string) (string, error)
	SendMessage(channelID string, message slack.Message, summaryText string,
	) (SentMessageInfo, error)
	UpdateMessage(
		channelID string, messageTS string, message slack.Message, summaryText string,
	) (SentMessageInfo, error)
	DeleteMessage(channelID string, messageTS string) error
	ReplaceCanvasContent(canvasID string, markdown string) error
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
	DeleteMessage(channelID string, timestamp string) (string, string, error)
	EditCanvas(params slack.EditCanvasParams) error
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

// The message must not have more than 50 blocks
func (c *client) SendMessage(
	channelID string,
	message slack.Message,
	summaryText string,
) (SentMessageInfo, error) {
	if len(message.Blocks.BlockSet) > 50 {
		return SentMessageInfo{}, fmt.Errorf(
			"message has too many blocks for Slack API (limit: 50, was: %v)",
			len(message.Blocks.BlockSet),
		)
	}

	log.Printf("\nSending message with summary: %s", summaryText)
	responseChannelID, timestamp, err := c.slackAPI.PostMessage(
		channelID,
		slack.MsgOptionBlocks(message.Blocks.BlockSet...),
		slack.MsgOptionText(summaryText, false),
		// The PR tracker canvas link would otherwise be expanded into a preview card
		// dwarfing the message it is a footer of.
		slack.MsgOptionDisableLinkUnfurl(),
		slack.MsgOptionDisableMediaUnfurl(),
	)
	if err != nil {
		return SentMessageInfo{}, fmt.Errorf("failed to send Slack message: %v", err)
	}
	log.Printf("Sent message to Slack channel: %s", channelID)

	return SentMessageInfo{
		ChannelID:  responseChannelID,
		Timestamp:  timestamp,
		JSONBlocks: parseSentJSONBlocks(message),
	}, nil
}

func (c *client) UpdateMessage(
	channelID string,
	messageTS string,
	message slack.Message,
	summaryText string,
) (SentMessageInfo, error) {
	log.Printf("Updating message with timestamp %s and summary: %s", messageTS, summaryText)
	_, _, _, err := c.slackAPI.UpdateMessage(
		channelID,
		messageTS,
		slack.MsgOptionBlocks(message.Blocks.BlockSet...),
		slack.MsgOptionText(summaryText, false),
		slack.MsgOptionDisableLinkUnfurl(),
		slack.MsgOptionDisableMediaUnfurl(),
	)
	if err != nil {
		return SentMessageInfo{}, fmt.Errorf("failed to update Slack message: %v", err)
	}
	log.Printf("Updated message in Slack channel: %s", channelID)

	return SentMessageInfo{
		ChannelID:  channelID,
		Timestamp:  messageTS,
		JSONBlocks: parseSentJSONBlocks(message),
	}, nil
}

func (c *client) DeleteMessage(channelID string, messageTS string) error {
	log.Printf("Deleting message with timestamp %s from channel %s", messageTS, channelID)
	_, _, err := c.slackAPI.DeleteMessage(channelID, messageTS)
	if err != nil {
		if strings.Contains(err.Error(), "message_not_found") {
			log.Printf("Message already deleted or not found, ignoring error")
			return nil
		}
		return fmt.Errorf("failed to delete Slack message: %v", err)
	}
	log.Printf("Deleted message from Slack channel: %s", channelID)
	return nil
}

// Replaces the whole content of the canvas: the change carries no section ID,
// which makes Slack apply it to the entire canvas.
func (c *client) ReplaceCanvasContent(canvasID string, markdown string) error {
	log.Printf("Replacing content of canvas %s with %d characters of markdown", canvasID, len(markdown))
	err := c.slackAPI.EditCanvas(slack.EditCanvasParams{
		CanvasID: canvasID,
		Changes: []slack.CanvasChange{{
			Operation: "replace",
			DocumentContent: slack.DocumentContent{
				Type:     "markdown",
				Markdown: markdown,
			},
		}},
	})
	if err != nil {
		return fmt.Errorf(
			"canvas update failed: check that the bot has canvases:write permission "+
				"and is invited to the channel where the canvas is: %v",
			err,
		)
	}
	log.Printf("Replaced content of canvas: %s", canvasID)
	return nil
}

func parseSentJSONBlocks(message slack.Message) []string {
	var sentJSONBlocks []string
	_, values, err := slack.UnsafeApplyMsgOptions(
		"", "", "", slack.MsgOptionBlocks(message.Blocks.BlockSet...),
	)
	if err == nil {
		if valuesBlocks, ok := values["blocks"]; ok && len(valuesBlocks) > 0 {
			sentJSONBlocks = valuesBlocks
		}
	} else {
		log.Printf("Warning: unable to parse sent JSON blocks: %v", err)
	}
	return sentJSONBlocks
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
