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
		GithubToken:                 utilities.GetRequiredEnv("INPUT_GITHUB-TOKEN"),
		Repository:                  utilities.GetRequiredEnv("GITHUB_REPOSITORY"),
		SlackBotToken:               utilities.GetRequiredEnv("INPUT_SLACK-BOT-TOKEN"),
		SlackChannelName:            utilities.GetRequiredEnv("INPUT_SLACK-CHANNEL-NAME"),
		OldPRThresholdHours:         utilities.GetEnvInt("INPUT_OLD-PR-THRESHOLD-HOURS"),
		SlackUserIdByGitHubUsername: utilities.GetStringMapping("INPUT_GITHUB-USER-SLACK-USER-ID-MAPPING"),
	}
}
