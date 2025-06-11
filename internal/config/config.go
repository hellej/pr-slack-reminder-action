package config

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/hellej/pr-slack-reminder-action/internal/config/utilities"
)

const (
	EnvGithubRepository              string = "GITHUB_REPOSITORY"
	InputGithubRepositories          string = "github-repositories"
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
	repository                  string
	repositories                []string
	SlackChannelName            string
	SlackChannelID              string
	SlackUserIdByGitHubUsername map[string]string
	ContentInputs               ContentInputs
}

func (c Config) GetGithubRepositories() []string {
	if len(c.repositories) > 0 {
		return c.repositories
	}
	return []string{c.repository}
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
	log.Print("Configuration:")
	log.Printf("repositories: %s", copy.GetGithubRepositories())
	log.Println(string(asJson))
}

func GetConfig() (Config, error) {
	repository, err1 := utilities.GetEnvRequired(EnvGithubRepository)
	githubToken, err2 := utilities.GetInputRequired(InputGithubToken)
	slackToken, err3 := utilities.GetInputRequired(InputSlackBotToken)
	mainListHeading, err4 := utilities.GetInputRequired(InputMainListHeading)
	oldPRsThresholdHours, err5 := utilities.GetInputInt(InputOldPRThresholdHours)
	slackUserIdByGitHubUsername, err6 := utilities.GetInputMapping(InputSlackUserIdByGitHubUsername)

	if err := selectNonNilError(err1, err2, err3, err4, err5, err6); err != nil {
		return Config{}, err
	}

	config := Config{
		repository:                  repository,
		repositories:                utilities.GetInputList(InputGithubRepositories),
		GithubToken:                 githubToken,
		SlackBotToken:               slackToken,
		SlackChannelName:            utilities.GetInput(InputSlackChannelName),
		SlackChannelID:              utilities.GetInput(InputSlackChannelID),
		SlackUserIdByGitHubUsername: slackUserIdByGitHubUsername,
		ContentInputs: ContentInputs{
			NoPRsMessage:        utilities.GetInput(InputNoPRsMessage),
			MainListHeading:     mainListHeading,
			OldPRsListHeading:   utilities.GetInput(InputOldPRsListHeading),
			OldPRThresholdHours: oldPRsThresholdHours,
		},
	}
	if config.SlackChannelID == "" && config.SlackChannelName == "" {
		return Config{}, fmt.Errorf(
			"either %s or %s must be set", InputSlackChannelID, InputSlackChannelName,
		)
	}
	if config.ContentInputs.OldPRThresholdHours != nil && config.ContentInputs.OldPRsListHeading == "" {
		return Config{}, fmt.Errorf(
			"if %s is set, %s must also be set", InputOldPRThresholdHours, InputOldPRsListHeading,
		)
	}
	return config, nil
}

func selectNonNilError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
