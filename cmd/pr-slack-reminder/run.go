package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/slackclient"
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

	// The message path and the canvas refresh are independent attempts: a failing reminder
	// says nothing about whether the canvas can be written, and a stale canvas is what this
	// feature exists to prevent. Their errors are collected instead of short-circuited.
	var messageErr, canvasErr, stateErr error
	var openPRs *githubclient.OpenPRsResult
	var stateToSave *state.State

	switch cfg.RunMode {
	case config.RunModePost:
		openPRs, stateToSave, messageErr = runPostMode(githubClient, slackClient, cfg, sentMessageHandler)
	case config.RunModeUpdate:
		stateToSave, messageErr = runUpdateMode(githubClient, slackClient, cfg, sentMessageHandler)
	default:
		return fmt.Errorf("unsupported run mode: %s", cfg.RunMode)
	}

	var canvasContentHash string
	if stateToSave != nil {
		canvasContentHash = stateToSave.CanvasContentHash
	}
	if cfg.CanvasEnabled() {
		canvasContentHash, canvasErr = refreshPRTrackerCanvas(
			githubClient, slackClient, openPRs, cfg, canvasContentHash,
		)
	}
	if canvasErr != nil {
		canvasErr = fmt.Errorf("PR tracker canvas refresh failed: %w", canvasErr)
	}
	if stateToSave != nil {
		stateToSave.CanvasContentHash = canvasContentHash
		stateErr = state.Save(cfg.StateFilePath, *stateToSave)
	}
	return errors.Join(messageErr, canvasErr, stateErr)
}

// Returns the open PRs it fetched, for the canvas refresh to share, and the state to save.
// The PRs are nil when the fetch failed, which leaves the canvas refresh to fetch its own.
// The state is nil on every path that leaves no message to track, so nothing is written.
func runPostMode(
	githubClient githubclient.Client,
	slackClient slackclient.Client,
	cfg config.Config,
	sentMessageHandler func(slackclient.SentMessageInfo) error,
) (*githubclient.OpenPRsResult, *state.State, error) {
	// The canvas shares this fetch, only with drafts switched on.
	fetched, err := findOpenPRs(githubClient, cfg, githubclient.PRFetchOptions{
		IncludeDrafts: cfg.CanvasEnabled(),
	})
	if err != nil {
		return nil, nil, err
	}

	// The message never shows drafts: they are dropped by the same predicate that keeps them
	// out of the fetch when the canvas is off, and before anything else sees the PRs.
	nonDraftPRs := utilities.Filter(fetched.PRs, func(pr githubclient.PR) bool {
		return !pr.GetDraft()
	})
	parsedPRs := prparser.ParsePRs(nonDraftPRs, cfg.ContentInputs)
	content := messagecontent.GetContent(parsedPRs, cfg.ContentInputs)
	if !content.HasPRs() && content.SummaryText == "" {
		log.Println("No PRs found and no message configured for this case, exiting")
		return &fetched, nil, nil
	}
	message, summaryText := messagebuilder.BuildMessage(content)

	sentMessageInfo, err := slackClient.SendMessage(cfg.SlackChannelID, message, summaryText)
	if err != nil {
		return &fetched, nil, err
	}

	postState := state.NewPostState(parsedPRs, sentMessageInfo)
	return &fetched, &postState, sentMessageHandler(sentMessageInfo)
}

func runUpdateMode(
	githubClient githubclient.Client,
	slackClient slackclient.Client,
	cfg config.Config,
	sentMessageHandler func(slackclient.SentMessageInfo) error,
) (*state.State, error) {
	loadedState, err := state.Load(
		context.Background(),
		githubClient,
		cfg.CurrentRepository,
		cfg.StateArtifactName,
		cfg.StateFilePath,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load state: %w", err)
	}
	if len(loadedState.PullRequests) == 0 {
		log.Println("No PRs to update in state, exiting")
		return loadedState, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), prFetchTimeout)
	defer cancel()
	prs, err := githubClient.GetPRs(ctx, loadedState.PullRequests, cfg.GetFiltersForRepository)
	if err != nil {
		return loadedState, err
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
		return loadedState, nil
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
		return loadedState, err
	}
	return loadedState, sentMessageHandler(sentMessageInfo)
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

// Returns a handler function that saves the sent Slack message blocks as a JSON file.
// This is useful in both dry-run mode of the action (TODO) and in integration tests.
func getSentMessageHandler(config config.Config) func(slackclient.SentMessageInfo) error {
	return func(sentMessageInfo slackclient.SentMessageInfo) error {
		if err := state.SaveSentSlackBlocksToFile(
			config.SentSlackBlocksFilePath, sentMessageInfo.JSONBlocks,
		); err != nil {
			return err
		}
		return nil
	}
}
