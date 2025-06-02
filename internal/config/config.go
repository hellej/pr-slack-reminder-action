package config

import "github.com/hellej/pr-slack-reminder-action/internal/utilities"

type Config struct {
	GithubToken                 string
	Repository                  string
	SlackBotToken               string
	SlackChannelName            string
	SlackChannelID              string
	OldPRThresholdHours         *int
	SlackUserIdByGitHubUsername *map[string]string
}

func GetConfig() Config {
	config := Config{
		Repository:                  utilities.GetEnvRequired("GITHUB_REPOSITORY"),
		GithubToken:                 utilities.GetInputRequired("github-token"),
		SlackBotToken:               utilities.GetInputRequired("slack-bot-token"),
		SlackChannelName:            utilities.GetInput("slack-channel-name"),
		SlackChannelID:              utilities.GetInput("slack-channel-id"),
		OldPRThresholdHours:         utilities.GetInputInt("old-pr-threshold-hours"),
		SlackUserIdByGitHubUsername: utilities.GetStringMapping("github-user-slack-user-id-mapping"),
	}
	if config.SlackChannelID == "" && config.SlackChannelName == "" {
		panic("Either slack-channel-id or slack-channel-name must be set")
	}

	return config
}
