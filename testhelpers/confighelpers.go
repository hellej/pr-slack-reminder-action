package testhelpers

import (
	"strconv"
	"strings"
	"testing"

	"github.com/hellej/pr-slack-reminder-action/internal/config"
)

func SetTestEnvironment(t *testing.T, c TestConfig, overrides *map[string]any) {
	t.Helper()
	setEnvFromConfig(t, c, overrides)
}

type TestConfig struct {
	config.Config
	Repository   string
	Repositories []string
	// GlobalFilters as a JSON string (instead of config.Filters struct)
	GlobalFiltersRaw string
	// RepositoryFilters as a JSON string
	// e.g. "test-repo: {\"labels\": [\"feature\", \"fix\"]}; test-repo2: {\"authors-ignore\": [\"alice\"]}"
	RepositoryFiltersRaw string
	// PRLinkRepoPrefixes as a string mapping
	// e.g. "test-repo: TR1/; test-repo2: TR2/"
	PRLinkRepoPrefixesRaw string
	GroupByRepository     bool
}

func GetDefaultConfigFull() TestConfig {
	oldPRsThresholdHours := 48
	slackUserIdByGithubUsername := map[string]string{
		"testuser": "U1234567890",
		"alice":    "U2234567890",
		"bob":      "U3234567890",
	}

	return TestConfig{
		Config: config.Config{
			GithubToken:             "SOME_TOKEN",
			SlackBotToken:           "SOME_TOKEN",
			RunMode:                 config.RunModePost,
			StateArtifactName:       "pr-slack-reminder-state",
			StateFilePath:           "/tmp/pr-slack-reminder-state.json",
			SentSlackBlocksFilePath: "/tmp/sent-slack-blocks.json",
			SlackChannelName:        "some-channel-name",
			ContentInputs: config.ContentInputs{
				NoPRsMessage:                "No open PRs found.",
				PRListHeading:               "There are <pr_count> open PRs 🚀",
				SlackUserIdByGitHubUsername: slackUserIdByGithubUsername,
				OldPRThresholdHours:         oldPRsThresholdHours,
			},
		},
		Repository:            "test-org/test-repo",
		Repositories:          []string{"test-org/test-repo"},
		GlobalFiltersRaw:      "{\"labels\": [\"feature\", \"fix\"], \"authors\": [\"alice\", \"stitch\"]}",
		RepositoryFiltersRaw:  "test-repo: {\"labels-ignore\": [\"label-to-ignore\"], \"authors-ignore\": [\"author-to-ignore\"]}",
		PRLinkRepoPrefixesRaw: "test-repo: some-repo-prefix/",
	}
}

func GetDefaultConfigMinimal() TestConfig {
	return TestConfig{
		Repository: "test-org/test-repo",
		Config: config.Config{
			GithubToken:             "SOME_TOKEN",
			SlackBotToken:           "SOME_TOKEN",
			RunMode:                 config.RunModePost,
			StateArtifactName:       "pr-slack-reminder-state",
			StateFilePath:           "/tmp/pr-slack-reminder-state.json",
			SentSlackBlocksFilePath: "/tmp/sent-slack-blocks.json",
			SlackChannelName:        "some-channel-name",
			ContentInputs: config.ContentInputs{
				PRListHeading: "There are <pr_count> open PRs 🚀",
			},
		},
	}
}

func setEnvFromConfig(t *testing.T, c TestConfig, overrides *map[string]any) {
	setEnv(t, overrides, config.EnvGithubRepository, c.Repository)
	setEnv(t, overrides, config.EnvSentSlackBlocksFilePath, c.SentSlackBlocksFilePath)
	setEnv(t, overrides, config.EnvStateFilePath, c.StateFilePath)

	setInputEnv(t, overrides, config.InputGithubRepositories, c.Repositories)
	setInputEnv(t, overrides, config.InputGithubToken, c.GithubToken)
	setInputEnv(t, overrides, config.InputSlackBotToken, c.SlackBotToken)
	setInputEnv(t, overrides, config.InputRunMode, string(c.RunMode))
	setInputEnv(t, overrides, config.InputStateArtifactName, c.StateArtifactName)
	setInputEnv(t, overrides, config.InputSlackChannelName, c.SlackChannelName)
	setInputEnv(t, overrides, config.InputSlackChannelID, c.SlackChannelID)
	setInputEnv(t, overrides, config.InputSlackUserIdByGitHubUsername, c.ContentInputs.SlackUserIdByGitHubUsername)
	setInputEnv(t, overrides, config.InputNoPRsMessage, c.ContentInputs.NoPRsMessage)
	setInputEnv(t, overrides, config.InputPRListHeading, c.ContentInputs.PRListHeading)
	setInputEnv(t, overrides, config.InputOldPRThresholdHours, c.ContentInputs.OldPRThresholdHours)
	setInputEnv(t, overrides, config.InputGlobalFilters, c.GlobalFiltersRaw)
	setInputEnv(t, overrides, config.InputRepositoryFilters, c.RepositoryFiltersRaw)
	setInputEnv(t, overrides, config.InputPRLinkRepoPrefixes, c.PRLinkRepoPrefixesRaw)
	setInputEnv(t, overrides, config.InputGroupByRepository, c.GroupByRepository)
}

func setEnv(t *testing.T, overrides *map[string]any, envName string, value any) {
	strValue := getValueAsString(t, overrides, envName, value)
	if strValue != nil {
		t.Setenv(envName, *strValue)
	}
}

func setInputEnv(t *testing.T, overrides *map[string]any, inputName string, value any) {
	strValue := getValueAsString(t, overrides, inputName, value)
	envName := inputNameAsEnv(inputName)
	if strValue != nil {
		t.Setenv(envName, *strValue)
	}
}

func getValueAsString(
	t *testing.T, overrides *map[string]any, inputName string, value any,
) *string {
	var strValue string
	if overrides != nil {
		if overrideValue, ok := (*overrides)[inputName]; ok {
			value = overrideValue
		}
	}
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case *map[string]string:
		strValue = mappingAsString(v)
	case map[string]string:
		strValue = mappingAsString(&v)
	case string:
		strValue = v
	case []string:
		strValue = listAsString(v)
	case int:
		strValue = strconv.Itoa(v)
	case *int:
		if v == nil {
			empty := ""
			return &empty
		}
		strValue = strconv.Itoa(*v)
	case bool:
		strValue = strconv.FormatBool(v)
	case config.RunMode:
		strValue = string(v)
	default:
		t.Fatalf("unsupported value type for setInputEnv: %T", value)
	}
	return &strValue
}

func listAsString(list []string) string {
	if list == nil {
		return ""
	}
	asString := ""
	for _, item := range list {
		if asString != "" {
			asString += ";"
		}
		asString += item
	}
	return asString
}

func mappingAsString(mapping *map[string]string) string {
	if mapping == nil {
		return ""
	}
	asString := ""
	for key, value := range *mapping {
		if asString != "" {
			asString += ";"
		}
		asString += key + ":" + value
	}
	return asString
}

func inputNameAsEnv(name string) string {
	e := strings.ReplaceAll(name, " ", "_")
	e = strings.ToUpper(e)
	return "INPUT_" + e
}
