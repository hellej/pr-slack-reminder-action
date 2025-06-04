package config

import "github.com/hellej/pr-slack-reminder-action/internal/utilities"

type ContentInputs struct {
	NoPRsMessage        string
	MainListHeading     string
	OldPRsListHeading   string
	OldPRThresholdHours *int
}

type Config struct {
	GithubToken                 string
	Repository                  string
	SlackBotToken               string
	SlackChannelName            string
	SlackChannelID              string
	SlackUserIdByGitHubUsername *map[string]string
	ContentInputs               ContentInputs
}

func GetConfig() Config {
	config := Config{
		Repository:                  utilities.GetEnvRequired("GITHUB_REPOSITORY"),
		GithubToken:                 utilities.GetInputRequired("github-token"),
		SlackBotToken:               utilities.GetInputRequired("slack-bot-token"),
		SlackChannelName:            utilities.GetInput("slack-channel-name"),
		SlackChannelID:              utilities.GetInput("slack-channel-id"),
		SlackUserIdByGitHubUsername: utilities.GetStringMapping("github-user-slack-user-id-mapping"),
		ContentInputs: ContentInputs{
			NoPRsMessage:        utilities.GetInput("no-prs-message"),
			MainListHeading:     utilities.GetInput("main-list-heading"),
			OldPRsListHeading:   utilities.GetInput("old-prs-list-heading"),
			OldPRThresholdHours: utilities.GetInputInt("old-pr-threshold-hours"),
		},
	}
	if config.SlackChannelID == "" && config.SlackChannelName == "" {
		panic("Either slack-channel-id or slack-channel-name must be set")
	}

	return config
}
