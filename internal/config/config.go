package config

import "github.com/hellej/pr-slack-reminder-action/internal/utilities"

type Config struct {
	GithubToken                 string
	Repository                  string
	SlackBotToken               string
	SlackChannelName            string
	OldPRThresholdHours         *int
	SlackUserIdByGitHubUsername *map[string]string
}

func GetConfig() Config {
	return Config{
		Repository:                  utilities.GetEnvRequired("GITHUB_REPOSITORY"),
		GithubToken:                 utilities.GetInputRequired("github-token"),
		SlackBotToken:               utilities.GetInputRequired("slack-bot-token"),
		SlackChannelName:            utilities.GetInputRequired("slack-channel-name"),
		OldPRThresholdHours:         utilities.GetInputInt("old-pr-threshold-hours"),
		SlackUserIdByGitHubUsername: utilities.GetStringMapping("github-user-slack-user-id-mapping"),
	}
}
