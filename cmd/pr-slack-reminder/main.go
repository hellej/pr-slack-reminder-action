package main

import (
	"log"

	composer "github.com/hellej/pr-slack-reminder-action/internal/composer"
	"github.com/hellej/pr-slack-reminder-action/internal/githubhelpers"
	"github.com/hellej/pr-slack-reminder-action/internal/slacknotifier"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

type Settings struct {
	GithubToken         string
	Repository          string
	SlackBotToken       string
	SlackChannelName    string
	OldPRThresholdHours int
}

func getSettings() Settings {
	return Settings{
		GithubToken:         utilities.GetRequiredEnv("GITHUB_TOKEN"),
		Repository:          utilities.GetRequiredEnv("GITHUB_REPOSITORY"),
		SlackBotToken:       utilities.GetRequiredEnv("SLACK_BOT_TOKEN"),
		SlackChannelName:    utilities.GetRequiredEnv("SLACK_CHANNEL_NAME"),
		OldPRThresholdHours: utilities.GetEnvIntOr("OLD_PR_THRESHOLD_HOURS", 365*24),
	}
}

func main() {
	log.Println("Starting PR Slack reminders action...")

	settings := getSettings()
	githubClient := githubhelpers.GetClient(settings.GithubToken)
	slackClient := slacknotifier.GetClient(settings.SlackBotToken)

	prs := githubhelpers.FetchOpenPRs(githubClient, settings.Repository)
	blocks, summaryText := composer.ComposeMessage(prs, settings.OldPRThresholdHours)
	slacknotifier.SendMessage(slackClient, settings.SlackChannelName, blocks, summaryText)
}
