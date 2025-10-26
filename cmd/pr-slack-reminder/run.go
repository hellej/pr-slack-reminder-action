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

	const prFetchTimeout = 60 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), prFetchTimeout)
	defer cancel()
	prs, err := githubClient.FetchOpenPRs(ctx, cfg.Repositories, cfg.GetFiltersForRepository)
	if err != nil {
		return err
	}
	parsedPRs := prparser.ParsePRs(prs, cfg.ContentInputs)
	content := messagecontent.GetContent(parsedPRs, cfg.ContentInputs)
	if !content.HasPRs() && content.SummaryText == "" {
		log.Println("No PRs found and no message configured for this case, exiting")
		return nil
	}
	blocks, summaryText := messagebuilder.BuildMessage(content)

	messageResponse, err := slackClient.SendMessage(cfg.SlackChannelID, blocks, summaryText)
	if err != nil {
		return err
	}

	if cfg.RunMode == config.RunModePost {
		if err := state.SavePostState(cfg.StateFilePath, parsedPRs, messageResponse); err != nil {
			return err
		}
	}

	return nil
}
