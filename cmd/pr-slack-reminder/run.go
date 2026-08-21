package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/slackclient"
	"github.com/hellej/pr-slack-reminder-action/internal/canvasbuilder"
	"github.com/hellej/pr-slack-reminder-action/internal/canvascontent"
	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/internal/messagebuilder"
	"github.com/hellej/pr-slack-reminder-action/internal/messagecontent"
	"github.com/hellej/pr-slack-reminder-action/internal/prparser"
	"github.com/hellej/pr-slack-reminder-action/internal/state"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

const prFetchTimeout = 60 * time.Second

func Run(
	getGitHubClient func(token, tokenForState string) githubclient.Client,
	getSlackClient func(token string) slackclient.Client,
) error {
	cfg, err := config.GetConfig()
	if err != nil {
		return fmt.Errorf("configuration error: %v", err)
	}
	cfg.Print()
	githubClient := getGitHubClient(cfg.GithubToken, cfg.GithubTokenForState)
	slackClient := getSlackClient(cfg.SlackBotToken)

	if cfg.SlackChannelID == "" {
		log.Println("Slack channel ID is not set, resolving it by name")
		channelID, err := slackClient.GetChannelIDByName(cfg.SlackChannelName)
		if err != nil {
			return fmt.Errorf("error getting channel ID by name: %v", err)
		}
		cfg.SlackChannelID = channelID
	}

	sentMessageHandler := getSentMessageHandler(cfg)
	canvasEnabled := cfg.PRTrackerCanvasID != ""

	// The message path and the canvas refresh are independent attempts: a failing reminder
	// says nothing about whether the canvas can be written, and a stale canvas is what this
	// feature exists to prevent. Their errors are collected instead of short-circuited.
	var messageErr, canvasErr error
	var canvasPRs githubclient.OpenPRsResult
	canvasPRsFetched := false

	switch cfg.RunMode {
	case config.RunModePost:
		var postResult postModeResult
		postResult, messageErr = runPostMode(githubClient, slackClient, cfg, sentMessageHandler)
		canvasPRs, canvasPRsFetched = postResult.fetched, postResult.prsFetched
	case config.RunModeUpdate:
		messageErr = runUpdateMode(githubClient, slackClient, cfg, sentMessageHandler)
		if canvasEnabled {
			// Update mode's message is state-tracked, so the canvas needs its own fetch of
			// what is open right now.
			canvasPRs, canvasErr = findOpenPRs(
				githubClient, cfg, githubclient.PRFetchOptions{IncludeDrafts: true},
			)
			canvasPRsFetched = canvasErr == nil
		}
	default:
		return fmt.Errorf("unsupported run mode: %s", cfg.RunMode)
	}

	if canvasEnabled && canvasPRsFetched {
		canvasErr = refreshPRTrackerCanvas(slackClient, cfg, canvasPRs)
	}
	if canvasErr != nil {
		canvasErr = fmt.Errorf("PR tracker canvas refresh failed: %w", canvasErr)
	}
	return errors.Join(messageErr, canvasErr)
}

// Refreshes the canvas from already fetched PRs, so that both run modes share one code path.
func refreshPRTrackerCanvas(
	slackClient slackclient.Client,
	cfg config.Config,
	fetched githubclient.OpenPRsResult,
) error {
	parsedPRs := prparser.ParsePRs(fetched.PRs, cfg.ContentInputs)
	content := canvascontent.GetContent(parsedPRs, cfg.ContentInputs, canvascontent.GetContentOptions{
		OpenPRsCapped: fetched.OpenPRsCapped,
		WIPPRsCapped:  fetched.DraftPRsCapped,
		GeneratedAt:   time.Now().UTC(),
	})
	return slackClient.ReplaceCanvasContent(cfg.PRTrackerCanvasID, canvasbuilder.BuildMarkdown(content))
}

func findOpenPRs(
	githubClient githubclient.Client,
	cfg config.Config,
	fetchOptions githubclient.PRFetchOptions,
) (githubclient.OpenPRsResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), prFetchTimeout)
	defer cancel()
	return githubClient.FindOpenPRs(ctx, cfg.Repositories, cfg.GetFiltersForRepository, fetchOptions)
}

// What the canvas refresh needs from a "post" run: the PRs it shares with the message path,
// and whether they were fetched at all. An empty PR list means "nothing open" only when the
// fetch succeeded, and refreshing on a failed fetch would wipe the canvas.
type postModeResult struct {
	fetched    githubclient.OpenPRsResult
	prsFetched bool
}

func runPostMode(
	githubClient githubclient.Client,
	slackClient slackclient.Client,
	cfg config.Config,
	sentMessageHandler func(slackclient.SentMessageInfo) error,
) (postModeResult, error) {
	// The canvas shares this fetch, only with drafts switched on.
	fetched, err := findOpenPRs(githubClient, cfg, githubclient.PRFetchOptions{
		IncludeDrafts: cfg.PRTrackerCanvasID != "",
	})
	if err != nil {
		return postModeResult{}, err
	}
	result := postModeResult{fetched: fetched, prsFetched: true}

	// The message never shows drafts: they are dropped by the same predicate that keeps them
	// out of the fetch when the canvas is off, and before anything else sees the PRs.
	nonDraftPRs := utilities.Filter(fetched.PRs, func(pr githubclient.PR) bool {
		return !pr.GetDraft()
	})
	parsedPRs := prparser.ParsePRs(nonDraftPRs, cfg.ContentInputs)
	content := messagecontent.GetContent(parsedPRs, cfg.ContentInputs)
	if !content.HasPRs() && content.SummaryText == "" {
		log.Println("No PRs found and no message configured for this case, exiting")
		return result, nil
	}
	message, summaryText := messagebuilder.BuildMessage(content)

	sentMessageInfo, err := slackClient.SendMessage(cfg.SlackChannelID, message, summaryText)
	if err != nil {
		return result, err
	}

	if err := state.SavePostState(cfg.StateFilePath, parsedPRs, sentMessageInfo); err != nil {
		return result, err
	}
	return result, sentMessageHandler(sentMessageInfo)
}

func runUpdateMode(
	githubClient githubclient.Client,
	slackClient slackclient.Client,
	cfg config.Config,
	sentMessageHandler func(slackclient.SentMessageInfo) error,
) error {
	loadedState, err := state.Load(
		context.Background(),
		githubClient,
		cfg.CurrentRepository,
		cfg.StateArtifactName,
		cfg.StateFilePath,
	)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}
	if len(loadedState.PullRequests) == 0 {
		log.Println("No PRs to update in state, exiting")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), prFetchTimeout)
	defer cancel()
	prs, err := githubClient.GetPRs(ctx, loadedState.PullRequests, cfg.GetFiltersForRepository)
	if err != nil {
		return err
	}

	parsedPRs := prparser.ParsePRs(prs, cfg.ContentInputs)
	content := messagecontent.GetContent(parsedPRs, cfg.ContentInputs)

	if !content.HasPRs() && content.SummaryText == "" {
		log.Println("All PRs from state have been filtered out or closed")
		log.Println("Deleting Slack message as no-prs-message input is not set")
		if err := slackClient.DeleteMessage(
			loadedState.SlackMessage.ChannelID,
			loadedState.SlackMessage.MessageTS,
		); err != nil {
			log.Printf("Warning: failed to delete message: %v", err)
		}
		return nil
	}
	if !content.HasPRs() && content.SummaryText != "" {
		log.Printf("All PRs from state have been filtered out or closed")
		log.Printf("Updating Slack message with no-prs-message: %s", content.SummaryText)
	}

	message, summaryText := messagebuilder.BuildMessage(content)

	sentMessageInfo, err := slackClient.UpdateMessage(
		loadedState.SlackMessage.ChannelID,
		loadedState.SlackMessage.MessageTS,
		message,
		summaryText,
	)
	if err != nil {
		return err
	}
	return sentMessageHandler(sentMessageInfo)
}

// Returns a handler function that saves the sent Slack message blocks as a JSON file.
// This is useful in both dry-run mode of the action (TODO) and in integration tests.
func getSentMessageHandler(config config.Config) func(slackclient.SentMessageInfo) error {
	return func(sentMessageInfo slackclient.SentMessageInfo) error {
		if err := state.SaveSentSlackBlocks(
			config.SentSlackBlocksFilePath, sentMessageInfo.JSONBlocks,
		); err != nil {
			return err
		}
		return nil
	}
}
