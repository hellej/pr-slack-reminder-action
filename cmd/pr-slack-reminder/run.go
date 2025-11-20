package main

import (
	"context"
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
)

func Run(
	getGitHubClient func(token string) githubclient.Client,
	getSlackClient func(token string) slackclient.Client,
) error {
	cfg, err := config.GetConfig()
	if err != nil {
		return fmt.Errorf("configuration error: %v", err)
	}
	cfg.Print()
	githubClient := getGitHubClient(cfg.GithubToken)
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

	switch cfg.RunMode {
	case config.RunModePost:
		return runPostMode(githubClient, slackClient, cfg, sentMessageHandler)
	case config.RunModeUpdate:
		return runUpdateMode(githubClient, slackClient, cfg, sentMessageHandler)
	default:
		return fmt.Errorf("unsupported run mode: %s", cfg.RunMode)
	}
}

func runPostMode(
	githubClient githubclient.Client,
	slackClient slackclient.Client,
	cfg config.Config,
	sentMessageHandler func(slackclient.SentMessageInfo) error,
) error {
	const prFetchTimeout = 60 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), prFetchTimeout)
	defer cancel()
	prs, err := githubClient.FindOpenPRs(ctx, cfg.Repositories, cfg.GetFiltersForRepository)
	if err != nil {
		return err
	}

	parsedPRs := prparser.ParsePRs(prs, cfg.ContentInputs)
	content := messagecontent.GetContent(parsedPRs, cfg.ContentInputs)
	if !content.HasPRs() && content.SummaryText == "" {
		log.Println("No PRs found and no message configured for this case, exiting")
		return nil
	}
	message, summaryText := messagebuilder.BuildMessage(content)

	sentMessageInfo, err := slackClient.SendMessage(cfg.SlackChannelID, message, summaryText)
	if err != nil {
		return err
	}

	if err := state.SavePostState(cfg.StateFilePath, parsedPRs, sentMessageInfo); err != nil {
		return err
	}
	return sentMessageHandler(sentMessageInfo)
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

	const prFetchTimeout = 60 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), prFetchTimeout)
	defer cancel()
	prs, err := githubClient.GetPRs(ctx, loadedState.PullRequests, cfg.GetFiltersForRepository)
	if err != nil {
		return err
	}

	parsedPRs := prparser.ParsePRs(prs, cfg.ContentInputs)
	content := messagecontent.GetContent(parsedPRs, cfg.ContentInputs)
	if !content.HasPRs() && content.SummaryText == "" {
		log.Println("No PRs found and no message configured for this case, exiting")
		return nil
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
