package main

import (
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

	prs := parser.ParsePRs(
		githubhelpers.FetchOpenPRs(githubClient, config.Repository),
	)
	content := content.GetContent(prs, config.OldPRThresholdHours)
	blocks, summaryText := composer.ComposeMessage(content)
	return slackhelpers.SendMessage(slackClient, config.SlackChannelName, blocks, summaryText)
}

func main() {
	err := run()
	if err != nil {
		log.Fatalf("%v", err)
	}
}
