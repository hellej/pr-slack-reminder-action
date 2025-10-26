// Package config handles GitHub Action input parsing and validation.
// It converts environment variables to structured configuration with
// support for repository-specific filters, user mappings, and content settings.
package config

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/hellej/pr-slack-reminder-action/internal/config/inputhelpers"

	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

const (
	EnvGithubRepository string = "GITHUB_REPOSITORY"

	InputGithubToken                 string = "github-token"
	InputSlackBotToken               string = "slack-bot-token"
	InputRunMode                     string = "mode"
	InputStateFilePath               string = "state-file-path"
	InputSlackChannelName            string = "slack-channel-name"
	InputSlackChannelID              string = "slack-channel-id"
	InputGithubRepositories          string = "github-repositories"
	InputGlobalFilters               string = "filters"
	InputRepositoryFilters           string = "repository-filters"
	InputSlackUserIdByGitHubUsername string = "github-user-slack-user-id-mapping"
	InputPRListHeading               string = "main-list-heading"
	InputNoPRsMessage                string = "no-prs-message"
	InputOldPRThresholdHours         string = "old-pr-threshold-hours"
	InputGroupByRepository           string = "group-by-repository"
	InputPRLinkRepoPrefixes          string = "pr-link-repo-prefixes"

	MaxRepositories int = 30

	DefaultRunMode       = RunModePost
	DefaultStateFilePath = ".pr-slack-reminder/state.json"
)

type Config struct {
	GithubToken   string
	SlackBotToken string

	RunMode       RunMode
	StateFilePath string

	SlackChannelName string
	SlackChannelID   string

	repository   string
	Repositories []models.Repository

	GlobalFilters     Filters
	RepositoryFilters map[string]Filters
	ContentInputs     ContentInputs
}

type ContentInputs struct {
	SlackUserIdByGitHubUsername map[string]string
	PRListHeading               string
	NoPRsMessage                string
	OldPRThresholdHours         int
	GroupByRepository           bool
	prLinkRepoPrefixes          map[string]string
}

func (contentInputs ContentInputs) GetPRLinkRepoPrefix(repo models.Repository) string {
	for _, key := range []string{repo.GetPath(), repo.Name} {
		if prefix, exists := contentInputs.prLinkRepoPrefixes[key]; exists {
			return prefix
		}
	}
	return ""
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
	githubToken, err1 := inputhelpers.GetInputRequired(InputGithubToken)
	slackToken, err2 := inputhelpers.GetInputRequired(InputSlackBotToken)
	runMode, err3 := getRunMode(InputRunMode)
	stateFilePath := inputhelpers.GetInputOr(InputStateFilePath, DefaultStateFilePath)
	slackChannelName := inputhelpers.GetInput(InputSlackChannelName)
	slackChannelID := inputhelpers.GetInput(InputSlackChannelID)
	repository, err4 := inputhelpers.GetEnvRequired(EnvGithubRepository)
	repositoryPaths := inputhelpers.GetInputList(InputGithubRepositories)
	globalFilters, err5 := GetGlobalFiltersFromInput(InputGlobalFilters)
	repositoryFilters, err6 := GetRepositoryFiltersFromInput(InputRepositoryFilters)
	slackUserIdByGitHubUsername, err7 := inputhelpers.GetInputMapping(InputSlackUserIdByGitHubUsername)
	mainListHeading := inputhelpers.GetInput(InputPRListHeading)
	noPRsMessage := inputhelpers.GetInput(InputNoPRsMessage)
	oldPRsThresholdHours, err8 := inputhelpers.GetInputInt(InputOldPRThresholdHours)
	groupByRepository, err9 := inputhelpers.GetInputBool(InputGroupByRepository)
	prLinkRepoPrefixes, err10 := inputhelpers.GetInputMapping(InputPRLinkRepoPrefixes)

	if err := selectNonNilError(err1, err2, err3, err4, err5, err6, err7, err8, err9, err10); err != nil {
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
		GithubToken:       githubToken,
		SlackBotToken:     slackToken,
		RunMode:           runMode,
		StateFilePath:     stateFilePath,
		SlackChannelName:  slackChannelName,
		SlackChannelID:    slackChannelID,
		repository:        repository,
		Repositories:      repositories,
		GlobalFilters:     globalFilters,
		RepositoryFilters: repositoryFilters,
		ContentInputs: ContentInputs{
			SlackUserIdByGitHubUsername: slackUserIdByGitHubUsername,
			PRListHeading:               mainListHeading,
			NoPRsMessage:                noPRsMessage,
			OldPRThresholdHours:         oldPRsThresholdHours,
			GroupByRepository:           groupByRepository,
			prLinkRepoPrefixes:          inputhelpers.UnquoteValues(prLinkRepoPrefixes),
		},
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
	if c.RunMode != RunModePost && c.RunMode != RunModeUpdate {
		return fmt.Errorf("invalid run mode: %s (expected '%s' or '%s')", c.RunMode, RunModePost, RunModeUpdate)
	}
	if len(c.Repositories) == 0 {
		return fmt.Errorf("at least one repository must be specified in %s or %s", EnvGithubRepository, InputGithubRepositories)
	}
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
	if err := validateRepositoryReferences(
		c.Repositories, c.ContentInputs.prLinkRepoPrefixes, "pr-link-repo-prefixes",
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

func selectNonNilError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
