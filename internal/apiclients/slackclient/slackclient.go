// Package slackclient provides Slack API integration for channel resolution by name,
// message posting with Block Kit formatting, and full canvas content replacement by canvas ID.
package slackclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/retry"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
	"github.com/slack-go/slack"
)

// See slackclient.spec.md § Behaviour.
var ErrMessageNotEditable = errors.New("message cannot be edited")

var notEditableMessageErrorCodes = []string{"message_not_found", "cant_update_message", "edit_window_closed"}

type SentMessageInfo struct {
	ChannelID    string
	Timestamp    string
	BlocksAsSent json.RawMessage
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
	return NewClient(slack.New(token), retry.DefaultPolicy())
}

func NewClient(slackAPI SlackAPI, retryPolicy retry.Policy) Client {
	return &client{slackAPI: slackAPI, retryPolicy: retryPolicy}
}

// represents the Slack API methods relevant to us from github.com/slack-go/slack
type SlackAPI interface {
	GetConversations(params *slack.GetConversationsParameters) ([]slack.Channel, string, error)
	PostMessageContext(ctx context.Context, channelID string, options ...slack.MsgOption) (string, string, error)
	UpdateMessageContext(
		ctx context.Context, channelID string, timestamp string, options ...slack.MsgOption,
	) (string, string, string, error)
	DeleteMessageContext(ctx context.Context, channelID string, timestamp string) (string, string, error)
	EditCanvasContext(ctx context.Context, params slack.EditCanvasParams) error
}

type client struct {
	slackAPI    SlackAPI
	retryPolicy retry.Policy
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

	sentBlocks, err := MarshalBlocksAsSent(message)
	if err != nil {
		return SentMessageInfo{}, err
	}

	log.Printf("\nSending message with summary: %s", summaryText)
	sentInfo, err := retry.TransientFailures(
		context.Background(), c.retryPolicy, "Slack post",
		func(attemptCtx context.Context) retry.AttemptResult[SentMessageInfo] {
			responseChannelID, timestamp, err := c.slackAPI.PostMessageContext(
				attemptCtx,
				channelID,
				slack.MsgOptionBlocks(message.Blocks.BlockSet...),
				slack.MsgOptionText(summaryText, false),
			)
			return retry.AttemptResult[SentMessageInfo]{
				Value:     SentMessageInfo{ChannelID: responseChannelID, Timestamp: timestamp, BlocksAsSent: sentBlocks},
				Err:       err,
				Transient: slackSurelyDidNotProcess(err),
			}
		},
	)
	if err != nil {
		return SentMessageInfo{}, fmt.Errorf("failed to send Slack message: %w", err)
	}
	log.Printf("Sent message to Slack channel: %s", channelID)
	return sentInfo, nil
}

func (c *client) UpdateMessage(
	channelID string,
	messageTS string,
	message slack.Message,
	summaryText string,
) (SentMessageInfo, error) {
	sentBlocks, err := MarshalBlocksAsSent(message)
	if err != nil {
		return SentMessageInfo{}, err
	}

	log.Printf("Updating message with timestamp %s and summary: %s", messageTS, summaryText)
	err = c.retryIdempotentCall("Slack update", func(attemptCtx context.Context) error {
		_, _, _, err := c.slackAPI.UpdateMessageContext(
			attemptCtx,
			channelID,
			messageTS,
			slack.MsgOptionBlocks(message.Blocks.BlockSet...),
			slack.MsgOptionText(summaryText, false),
		)
		return err
	})
	if err != nil {
		return SentMessageInfo{}, WrapUpdateMessageError(err)
	}
	log.Printf("Updated message in Slack channel: %s", channelID)

	return SentMessageInfo{
		ChannelID:    channelID,
		Timestamp:    messageTS,
		BlocksAsSent: sentBlocks,
	}, nil
}

// See slackclient.spec.md § Oddities.
func WrapUpdateMessageError(err error) error {
	var slackError slack.SlackErrorResponse
	if errors.As(err, &slackError) && slices.Contains(notEditableMessageErrorCodes, slackError.Err) {
		return fmt.Errorf("failed to update Slack message: %w: %w", ErrMessageNotEditable, err)
	}
	return fmt.Errorf("failed to update Slack message: %w", err)
}

func (c *client) DeleteMessage(channelID string, messageTS string) error {
	log.Printf("Deleting message with timestamp %s from channel %s", messageTS, channelID)
	err := c.retryIdempotentCall("Slack delete", func(attemptCtx context.Context) error {
		_, _, err := c.slackAPI.DeleteMessageContext(attemptCtx, channelID, messageTS)
		return err
	})
	if err != nil {
		if strings.Contains(err.Error(), "message_not_found") {
			log.Printf("Message already deleted or not found, ignoring error")
			return nil
		}
		return fmt.Errorf("failed to delete Slack message: %w", err)
	}
	log.Printf("Deleted message from Slack channel: %s", channelID)
	return nil
}

// Replaces the whole content of the canvas: the change carries no section ID,
// which makes Slack apply it to the entire canvas.
func (c *client) ReplaceCanvasContent(canvasID string, markdown string) error {
	log.Printf("Replacing content of canvas %s with %d characters of markdown", canvasID, len(markdown))
	err := c.retryIdempotentCall("Slack canvas edit", func(attemptCtx context.Context) error {
		return c.slackAPI.EditCanvasContext(attemptCtx, slack.EditCanvasParams{
			CanvasID: canvasID,
			Changes: []slack.CanvasChange{{
				Operation: "replace",
				DocumentContent: slack.DocumentContent{
					Type:     "markdown",
					Markdown: markdown,
				},
			}},
		})
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

// See slackclient.spec.md § Behaviour.
func MarshalBlocksAsSent(message slack.Message) (json.RawMessage, error) {
	blocks, err := json.Marshal(message.Blocks.BlockSet)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Slack message blocks: %w", err)
	}
	return blocks, nil
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

func (c *client) retryIdempotentCall(apiName string, call func(attemptCtx context.Context) error) error {
	_, err := retry.TransientFailures(
		context.Background(), c.retryPolicy, apiName,
		func(attemptCtx context.Context) retry.AttemptResult[struct{}] {
			err := call(attemptCtx)
			return retry.AttemptResult[struct{}]{Err: err, Transient: isTransientForIdempotentCall(err)}
		},
	)
	return err
}

// See slackclient.spec.md § Behaviour.
var errorCodesSlackRejectsUnprocessed = []string{"ratelimited", "service_unavailable"}

var errorCodesWorthRetryingAnIdempotentCall = []string{"internal_error", "fatal_error", "request_timeout"}

func slackSurelyDidNotProcess(err error) bool {
	var rateLimitedError *slack.RateLimitedError
	if errors.As(err, &rateLimitedError) {
		return true
	}
	var statusCodeError slack.StatusCodeError
	if errors.As(err, &statusCodeError) && statusCodeError.Code == http.StatusTooManyRequests {
		return true
	}
	var slackError slack.SlackErrorResponse
	if errors.As(err, &slackError) && slices.Contains(errorCodesSlackRejectsUnprocessed, slackError.Err) {
		return true
	}
	var connectionError *net.OpError
	return errors.As(err, &connectionError) && connectionError.Op == "dial"
}

func isTransientForIdempotentCall(err error) bool {
	if slackSurelyDidNotProcess(err) {
		return true
	}
	var noResponseError *url.Error
	if errors.As(err, &noResponseError) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var statusCodeError slack.StatusCodeError
	if errors.As(err, &statusCodeError) && statusCodeError.Code >= http.StatusInternalServerError {
		return true
	}
	var slackError slack.SlackErrorResponse
	return errors.As(err, &slackError) && slices.Contains(errorCodesWorthRetryingAnIdempotentCall, slackError.Err)
}
