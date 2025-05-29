package main

import (
	"log"

	"github.com/hellej/pr-slack-reminder-action/internal/composer"
	"github.com/hellej/pr-slack-reminder-action/internal/content"
	"github.com/hellej/pr-slack-reminder-action/internal/githubhelpers"
	"github.com/hellej/pr-slack-reminder-action/internal/slackhelpers"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

type Settings struct {
	GithubToken                 string
	Repository                  string
	SlackBotToken               string
	SlackChannelName            string
	OldPRThresholdHours         *int
	SlackUserIdByGitHubUsername *map[string]string
}

func getSettings() Settings {
	return Settings{
		GithubToken:                 utilities.GetRequiredEnv("GITHUB_TOKEN"),
		Repository:                  utilities.GetRequiredEnv("GITHUB_REPOSITORY"),
		SlackBotToken:               utilities.GetRequiredEnv("SLACK_BOT_TOKEN"),
		SlackChannelName:            utilities.GetRequiredEnv("SLACK_CHANNEL_NAME"),
		OldPRThresholdHours:         utilities.GetEnvInt("OLD_PR_THRESHOLD_HOURS"),
		SlackUserIdByGitHubUsername: utilities.GetStringMapping("SLACK_USER_ID_BY_GITHUB_USERNAME"),
	}
}

func main() {
	log.Println("Starting PR Slack reminders action...")

	settings := getSettings()
	githubClient := githubhelpers.GetClient(settings.GithubToken)
	slackClient := slackhelpers.GetClient(settings.SlackBotToken)

	prs := githubhelpers.FetchOpenPRs(githubClient, settings.Repository)
	content := content.GetContent(prs, settings.OldPRThresholdHours)
	blocks, summaryText := composer.ComposeMessage(content)
	slackhelpers.SendMessage(slackClient, settings.SlackChannelName, blocks, summaryText)
}
