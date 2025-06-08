package config

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/hellej/pr-slack-reminder-action/internal/config/utilities"
)

const (
	EnvGithubRepository              string = "GITHUB_REPOSITORY"
	InputGithubToken                 string = "github-token"
	InputSlackBotToken               string = "slack-bot-token"
	InputSlackChannelName            string = "slack-channel-name"
	InputSlackChannelID              string = "slack-channel-id"
	InputSlackUserIdByGitHubUsername string = "github-user-slack-user-id-mapping"
	InputNoPRsMessage                string = "no-prs-message"
	InputMainListHeading             string = "main-list-heading"
	InputOldPRsListHeading           string = "old-prs-list-heading"
	InputOldPRThresholdHours         string = "old-pr-threshold-hours"
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
		Repository:                  utilities.GetEnvRequired(EnvGithubRepository),
		GithubToken:                 utilities.GetInputRequired(InputGithubToken),
		SlackBotToken:               utilities.GetInputRequired(InputSlackBotToken),
		SlackChannelName:            utilities.GetInput(InputSlackChannelName),
		SlackChannelID:              utilities.GetInput(InputSlackChannelID),
		SlackUserIdByGitHubUsername: utilities.GetInputMapping(InputSlackUserIdByGitHubUsername),
		ContentInputs: ContentInputs{
			NoPRsMessage:        utilities.GetInput(InputNoPRsMessage),
			MainListHeading:     utilities.GetInputRequired(InputMainListHeading),
			OldPRsListHeading:   utilities.GetInput(InputOldPRsListHeading),
			OldPRThresholdHours: utilities.GetInputInt(InputOldPRThresholdHours),
		},
	}
	if config.SlackChannelID == "" && config.SlackChannelName == "" {
		log.Panic("Either slack-channel-id or slack-channel-name must be set")
	}
	if config.ContentInputs.OldPRThresholdHours != nil && config.ContentInputs.OldPRsListHeading == "" {
		log.Panic("If old-pr-threshold-hours is set, old-prs-list-heading must also be set")
	}
	return config
}
