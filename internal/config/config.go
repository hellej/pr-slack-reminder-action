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
	InputGlobalFilters               string = "filters"
	InputRepositoryFilters           string = "repository-filters"
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
	Repositories                []Repository
	SlackChannelName            string
	SlackChannelID              string
	SlackUserIdByGitHubUsername map[string]string
	ContentInputs               ContentInputs
	GlobalFilters               Filters
	RepositoryFilters           map[string]Filters
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
	log.Println(string(asJson))
}

func GetConfig() (Config, error) {
	repository, err1 := utilities.GetEnvRequired(EnvGithubRepository)
	githubToken, err2 := utilities.GetInputRequired(InputGithubToken)
	slackToken, err3 := utilities.GetInputRequired(InputSlackBotToken)
	mainListHeading, err4 := utilities.GetInputRequired(InputMainListHeading)
	oldPRsThresholdHours, err5 := utilities.GetInputInt(InputOldPRThresholdHours)
	slackUserIdByGitHubUsername, err6 := utilities.GetInputMapping(InputSlackUserIdByGitHubUsername)
	globalFilters, err7 := GetGlobalFiltersFromInput(InputGlobalFilters)
	repositoryFilters, err8 := GetRepositoryFiltersFromInput(InputRepositoryFilters)

	if err := selectNonNilError(err1, err2, err3, err4, err5, err6, err7, err8); err != nil {
		return Config{}, err
	}

	repositoryPaths := utilities.GetInputList(InputGithubRepositories)
	if len(repositoryPaths) == 0 {
		repositoryPaths = []string{repository}
	}
	repositories := make([]Repository, len(repositoryPaths))
	for i, repoPath := range repositoryPaths {
		repo, err := parseRepository(repoPath)
		if err != nil {
			return Config{}, fmt.Errorf("invalid repositories input: %v", err)
		}
		repositories[i] = repo
	}

	config := Config{
		repository:                  repository,
		Repositories:                repositories,
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
		GlobalFilters:     globalFilters,
		RepositoryFilters: repositoryFilters,
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
