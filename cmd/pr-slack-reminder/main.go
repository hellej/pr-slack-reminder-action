package main

import (
	"log"

	"github.com/hellej/pr-slack-reminder-action/internal/githubhelpers"
	composer "github.com/hellej/pr-slack-reminder-action/internal/message-composer"
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
		OldPRThresholdHours: utilities.GetEnvInt("OLD_PR_THRESHOLD_HOURS", 44),
	}
}

func main() {
	log.Println("Starting PR Slack reminders action...")

	settings := getSettings()

	githubClient := githubhelpers.GetClient(settings.GithubToken)
	slackClient := slacknotifier.GetClient(settings.SlackBotToken)

	prs := githubhelpers.FetchOpenPRs(githubClient, settings.Repository)

	blocks, summaryText := composer.ComposeMessage(prs)
	slackErr := slacknotifier.SendMessage(slackClient, settings.SlackChannelName, blocks, summaryText)

	if slackErr != nil {
		log.Fatalf("Error sending message to Slack: %v", slackErr)
	}

}
