// Package config handles GitHub Action input parsing and validation.
// It converts environment variables to structured configuration with
// support for repository-specific filters, user mappings, and content settings.
package config

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/hellej/pr-slack-reminder-action/internal/config/inputhelpers"

	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

const (
	EnvGithubRepository        string = "GITHUB_REPOSITORY"
	EnvSentSlackBlocksFilePath string = "SENT_SLACK_BLOCKS_FILE_PATH"
	EnvStateFilePath           string = "STATE_FILE_PATH"

	InputSlackBotToken               string = "slack-bot-token"
	InputGithubToken                 string = "github-token"
	InputGithubTokenForState         string = "github-token-for-state"
	InputRunMode                     string = "run-mode"
	InputStateArtifactName           string = "state-artifact-name"
	InputSlackChannelName            string = "slack-channel-name"
	InputSlackChannelID              string = "slack-channel-id"
	InputGithubRepositories          string = "github-repositories"
	InputGlobalFilters               string = "filters"
	InputRepositoryFilters           string = "repository-filters"
	InputSlackUserIdByGitHubUsername string = "github-user-slack-user-id-mapping"
	InputPRListHeading               string = "pr-list-heading"
	InputNoPRsMessage                string = "no-prs-message"
	InputOldPRThresholdHours         string = "old-pr-threshold-hours"
	InputGroupByRepository           string = "group-by-repository"
	InputPRTrackerCanvasLink         string = "pr-tracker-canvas-link"

	MaxRepositories int = 30

	DefaultRunMode                 = RunModePost
	DefaultStateFilePath           = "pr-slack-reminder-state.json"
	DefaultSentSlackBlocksFilePath = "pr-slack-reminder-sent-blocks.json"
)

type Config struct {
	SlackBotToken       string
	GithubToken         string
	GithubTokenForState string

	RunMode                 RunMode
	StateArtifactName       string
	StateFilePath           string
	SentSlackBlocksFilePath string

	SlackChannelName string
	SlackChannelID   string

	CurrentRepository models.Repository
	Repositories      []models.Repository

	GlobalFilters     Filters
	RepositoryFilters map[string]Filters
	ContentInputs     ContentInputs

	PRTrackerCanvasID string
}

func (c Config) CanvasEnabled() bool {
	return c.PRTrackerCanvasID != ""
}

type ContentInputs struct {
	SlackUserIdByGitHubUsername map[string]string
	PRListHeading               string
	NoPRsMessage                string
	OldPRThresholdHours         int
	GroupByRepository           bool
}

func (c Config) Print() {
	copy := c
	if copy.SlackBotToken != "" {
		copy.SlackBotToken = "XXXXX"
	}
	if copy.GithubToken != "" {
		copy.GithubToken = "XXXXX"
	}
	if copy.GithubTokenForState != "" {
		copy.GithubTokenForState = "XXXXX"
	}
	asJson, _ := json.MarshalIndent(copy, "", "  ")
	log.Print("Configuration:")
	log.Println(string(asJson))
}

func GetConfig() (Config, error) {
	slackToken, err1 := inputhelpers.GetInputRequired(InputSlackBotToken)
	githubToken, err2 := inputhelpers.GetInputRequired(InputGithubToken)
	githubTokenForState := inputhelpers.GetInput(InputGithubTokenForState)

	runMode, err3 := getRunMode(InputRunMode)
	stateArtifactName := inputhelpers.GetInput(InputStateArtifactName)
	stateFilePath := cmp.Or(inputhelpers.GetEnv(EnvStateFilePath), DefaultStateFilePath)
	sentSlackBlocksFilePath := cmp.Or(
		inputhelpers.GetEnv(EnvSentSlackBlocksFilePath), DefaultSentSlackBlocksFilePath,
	)

	slackChannelName := inputhelpers.GetInput(InputSlackChannelName)
	slackChannelID := inputhelpers.GetInput(InputSlackChannelID)
	repository, err4 := inputhelpers.GetEnvRequired(EnvGithubRepository)
	currentRepository, err5 := models.ParseRepository(repository)
	repositoryPaths := inputhelpers.GetInputList(InputGithubRepositories)
	globalFilters, err6 := GetGlobalFiltersFromInput(InputGlobalFilters)
	repositoryFilters, err7 := GetRepositoryFiltersFromInput(InputRepositoryFilters)
	slackUserIdByGitHubUsername, err8 := inputhelpers.GetInputMapping(InputSlackUserIdByGitHubUsername)
	prListHeading := inputhelpers.GetInput(InputPRListHeading)
	noPRsMessage := inputhelpers.GetInput(InputNoPRsMessage)
	oldPRsThresholdHours, err9 := inputhelpers.GetInputInt(InputOldPRThresholdHours)
	groupByRepository, err10 := inputhelpers.GetInputBool(InputGroupByRepository)
	prTrackerCanvasURL := inputhelpers.GetInput(InputPRTrackerCanvasLink)
	prTrackerCanvasID, err11 := getCanvasIDFromLink(prTrackerCanvasURL)

	if err := errors.Join(
		err1, err2, err3, err4, err5, err6, err7, err8, err9, err10, err11,
	); err != nil {
		return Config{}, err
	}

	if len(repositoryPaths) == 0 {
		repositoryPaths = []string{repository}
	}

	repositories, err := utilities.MapWithError(repositoryPaths, func(repoPath string) (models.Repository, error) {
		return models.ParseRepository(repoPath)
	})
	if err != nil {
		return Config{}, fmt.Errorf("invalid repositories input: %v", err)
	}

	config := Config{
		SlackBotToken:           slackToken,
		GithubToken:             githubToken,
		GithubTokenForState:     githubTokenForState,
		RunMode:                 runMode,
		StateArtifactName:       stateArtifactName,
		StateFilePath:           stateFilePath,
		SentSlackBlocksFilePath: sentSlackBlocksFilePath,
		SlackChannelName:        slackChannelName,
		SlackChannelID:          slackChannelID,
		CurrentRepository:       currentRepository,
		Repositories:            repositories,
		GlobalFilters:           globalFilters,
		RepositoryFilters:       repositoryFilters,
		ContentInputs: ContentInputs{
			SlackUserIdByGitHubUsername: slackUserIdByGitHubUsername,
			PRListHeading:               prListHeading,
			NoPRsMessage:                noPRsMessage,
			OldPRThresholdHours:         oldPRsThresholdHours,
			GroupByRepository:           groupByRepository,
		},
		PRTrackerCanvasID: prTrackerCanvasID,
	}

	if err := config.validate(); err != nil {
		return Config{}, err
	}

	return config, nil
}

func (c Config) GetFiltersForRepository(repo models.Repository) Filters {
	for _, key := range []string{repo.GetPath(), repo.Name} {
		if filters, exists := c.RepositoryFilters[key]; exists {
			return filters
		}
	}
	return c.GlobalFilters
}

// validate performs post-construction validation of business rules for Config.
// It validates repository limits, Slack channel requirements and repository names.
func (c Config) validate() error {
	if c.SlackChannelID == "" && c.SlackChannelName == "" {
		return fmt.Errorf("either %s or %s must be set", InputSlackChannelID, InputSlackChannelName)
	}
	if len(c.Repositories) > MaxRepositories {
		return fmt.Errorf("too many repositories: maximum of %d repositories allowed, got %d", MaxRepositories, len(c.Repositories))
	}
	if err := c.validateRepositoryNames(); err != nil {
		return err
	}
	if err := c.validateHeadingOptions(); err != nil {
		return err
	}
	if err := c.validateStateArtifactName(); err != nil {
		return err
	}

	return nil
}

func (c Config) validateRepositoryNames() error {
	if err := validateDuplicateRepositories(c.Repositories); err != nil {
		return err
	}
	if err := validateRepositoryReferences(
		c.Repositories, c.RepositoryFilters, "repository-filters",
	); err != nil {
		return err
	}
	return nil
}

func validateDuplicateRepositories(repositories []models.Repository) error {
	repositoryPaths := make(map[string]bool, len(repositories))
	for _, repo := range repositories {
		if repositoryPaths[repo.GetPath()] {
			return fmt.Errorf("duplicate repository '%s' found in github-repositories", repo.GetPath())
		}
		repositoryPaths[repo.GetPath()] = true
	}
	return nil
}

// validateRepositoryReferences checks if any repository name appears multiple times
// across different owners, which would make repository-specific configurations ambiguous.
func validateRepositoryReferences[V any](
	repositories []models.Repository,
	repoMapping map[string]V,
	inputName string,
) error {
	for repoNameOrPath := range repoMapping {
		matches := utilities.Filter(repositories, func(r models.Repository) bool {
			return r.GetPath() == repoNameOrPath || r.Name == repoNameOrPath
		})

		switch len(matches) {
		case 1:
			continue
		case 0:
			return fmt.Errorf(
				"%s contains entry for '%s' which does not match any repository",
				inputName,
				repoNameOrPath,
			)
		default: // matches > 1
			return fmt.Errorf(
				"%s contains ambiguous entry for '%s' which matches "+
					"multiple repositories (needs owner/repo format)",
				inputName,
				repoNameOrPath,
			)
		}
	}
	return nil
}

func (c Config) validateHeadingOptions() error {
	if !c.ContentInputs.GroupByRepository && c.ContentInputs.PRListHeading == "" {
		return fmt.Errorf("%s is required when group-by-repository is false", InputPRListHeading)
	}
	return nil
}

func (c Config) validateStateArtifactName() error {
	if c.RunMode == RunModeUpdate && c.StateArtifactName == "" {
		return fmt.Errorf("%s is required when run mode is '%s'", InputStateArtifactName, RunModeUpdate)
	}
	return nil
}
