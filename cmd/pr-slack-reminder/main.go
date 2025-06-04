package main

import (
	"fmt"
	"log"

	"github.com/hellej/pr-slack-reminder-action/internal/composer"
	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/internal/content"
	"github.com/hellej/pr-slack-reminder-action/internal/githubhelpers"
	"github.com/hellej/pr-slack-reminder-action/internal/parser"
	"github.com/hellej/pr-slack-reminder-action/internal/slackhelpers"
)

func run() error {
	log.Println("Starting PR Slack reminder action")

	config := config.GetConfig()
	githubClient := githubhelpers.GetClient(config.GithubToken)
	slackClient := slackhelpers.GetClient(config.SlackBotToken)

	if config.SlackChannelID == "" {
		log.Println("Slack channel ID is not set, resolving it by name")
		channelID, err := slackhelpers.GetChannelIDByName(slackClient, config.SlackChannelName)
		if err != nil {
			return fmt.Errorf("error getting channel ID by name: %v", err)
		}
		config.SlackChannelID = channelID
	}

	prs := parser.ParsePRs(
		githubhelpers.FetchOpenPRs(githubClient, config.Repository),
		config.SlackUserIdByGitHubUsername,
	)
	content := content.GetContent(prs, config.ContentInputs)
	if !content.HasPRs() && content.SummaryText == "" {
		log.Println("No PRs found and no message configured for this case, exiting")
		return nil
	}
	blocks, summaryText := composer.ComposeMessage(content)
	return slackhelpers.SendMessage(slackClient, config.SlackChannelID, blocks, summaryText)
}

func main() {
	err := run()
	if err != nil {
		log.Fatalf("%v", err)
	}
}
