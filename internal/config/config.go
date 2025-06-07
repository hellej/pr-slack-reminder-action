package config

import (
	"encoding/json"
	"fmt"

	"github.com/hellej/pr-slack-reminder-action/internal/config/utilities"
)

type ContentInputs struct {
	NoPRsMessage        string
	MainListHeading     string
	OldPRsListHeading   string
	OldPRThresholdHours *int
}

type Config struct {
	GithubToken                 string
	SlackBotToken               string
	Repository                  string
	SlackChannelName            string
	SlackChannelID              string
	SlackUserIdByGitHubUsername *map[string]string
	ContentInputs               ContentInputs
}

func (c Config) Print() {
	copy := c
	if copy.GithubToken != "" {
		copy.GithubToken = "XXXXX"
	}
	if copy.SlackBotToken != "" {
		copy.SlackBotToken = "XXXXX"
	}
	asJson, _ := json.MarshalIndent(copy, "", "  ")
	fmt.Println("Configuration:")
	fmt.Println(string(asJson))
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
			MainListHeading:     utilities.GetInputRequired("main-list-heading"),
			OldPRsListHeading:   utilities.GetInput("old-prs-list-heading"),
			OldPRThresholdHours: utilities.GetInputInt("old-pr-threshold-hours"),
		},
	}
	if config.SlackChannelID == "" && config.SlackChannelName == "" {
		panic("Either slack-channel-id or slack-channel-name must be set")
	}

	return config
}
