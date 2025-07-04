package main

import (
	"fmt"
	"log"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/slackclient"
	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/internal/messagebuilder"
	"github.com/hellej/pr-slack-reminder-action/internal/messagecontent"
	"github.com/hellej/pr-slack-reminder-action/internal/prparser"
)

func Run(
	getGitHubClient func(token string) githubclient.Client,
	getSlackClient func(token string) slackclient.Client,
) error {
	config, err := config.GetConfig()
	if err != nil {
		return fmt.Errorf("configuration error: %v", err)
	}
	config.Print()
	githubClient := getGitHubClient(config.GithubToken)
	slackClient := getSlackClient(config.SlackBotToken)

	if config.SlackChannelID == "" {
		log.Println("Slack channel ID is not set, resolving it by name")
		channelID, err := slackClient.GetChannelIDByName(config.SlackChannelName)
		if err != nil {
			return fmt.Errorf("error getting channel ID by name: %v", err)
		}
		config.SlackChannelID = channelID
	}

	prs, err := githubClient.FetchOpenPRs(config.Repositories, config.GlobalFilters, config.RepositoryFilters)
	if err != nil {
		return err
	}
	parsedPRs := prparser.ParsePRs(prs, config.SlackUserIdByGitHubUsername)
	content := messagecontent.GetContent(parsedPRs, config.ContentInputs)
	if !content.HasPRs() && content.SummaryText == "" {
		log.Println("No PRs found and no message configured for this case, exiting")
		return nil
	}
	blocks, summaryText := messagebuilder.BuildMessage(content)
	return slackClient.SendMessage(config.SlackChannelID, blocks, summaryText)
}
