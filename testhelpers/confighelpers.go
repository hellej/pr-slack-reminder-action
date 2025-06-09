package testhelpers

import (
	"strconv"
	"strings"
	"testing"

	"github.com/hellej/pr-slack-reminder-action/internal/config"
)

func SetTestEnvironment(t *testing.T, c config.Config, overrides *map[string]any) {
	t.Helper()
	setEnvFromConfig(t, c, overrides)
}

func GetDefaultConfigFull() config.Config {
	oldPRsThresholdHours := 48
	slackUserIdByGithubUsername := map[string]string{
		"testuser": "U1234567890",
		"alice":    "U2234567890",
		"bob":      "U3234567890",
	}

	return config.Config{
		Repository:                  "test-org/test-repo",
		GithubToken:                 "SOME_TOKEN",
		SlackBotToken:               "SOME_TOKEN",
		SlackChannelName:            "some-channel-name",
		SlackUserIdByGitHubUsername: &slackUserIdByGithubUsername,
		ContentInputs: config.ContentInputs{
			NoPRsMessage:        "No open PRs found.",
			MainListHeading:     "There are <pr_count> open PRs 🚀",
			OldPRsListHeading:   "Old PRs 🚨",
			OldPRThresholdHours: &oldPRsThresholdHours,
		},
	}
}

func GetDefaultConfigMinimal() config.Config {
	return config.Config{
		Repository:       "test-org/test-repo",
		GithubToken:      "SOME_TOKEN",
		SlackBotToken:    "SOME_TOKEN",
		SlackChannelName: "some-channel-name",
		ContentInputs: config.ContentInputs{
			MainListHeading: "There are <pr_count> open PRs 🚀",
		},
	}
}

func setEnvFromConfig(t *testing.T, c config.Config, overrides *map[string]any) {
	t.Setenv(config.EnvGithubRepository, c.Repository)
	setInputEnv(t, overrides, config.InputGithubToken, c.GithubToken)
	setInputEnv(t, overrides, config.InputSlackBotToken, c.SlackBotToken)
	setInputEnv(t, overrides, config.InputSlackChannelName, c.SlackChannelName)
	setInputEnv(t, overrides, config.InputSlackChannelID, c.SlackChannelID)
	setInputEnv(t, overrides, config.InputSlackUserIdByGitHubUsername, c.SlackUserIdByGitHubUsername)
	setInputEnv(t, overrides, config.InputNoPRsMessage, c.ContentInputs.NoPRsMessage)
	setInputEnv(t, overrides, config.InputMainListHeading, c.ContentInputs.MainListHeading)
	setInputEnv(t, overrides, config.InputOldPRsListHeading, c.ContentInputs.OldPRsListHeading)
	setInputEnv(t, overrides, config.InputOldPRThresholdHours, c.ContentInputs.OldPRThresholdHours)
}

func setInputEnv(t *testing.T, overrides *map[string]interface{}, inputName string, value interface{}) {
	var strValue string
	if overrides != nil {
		if overrideValue, ok := (*overrides)[inputName]; ok {
			value = overrideValue
		}
	}
	if value == nil {
		return
	}
	switch v := value.(type) {
	case *map[string]string:
		strValue = mappingAsString(v)
	case string:
		strValue = v
	case int:
		strValue = strconv.Itoa(v)
	case *int:
		if v == nil {
			t.Setenv(inputNameAsEnv(inputName), "")
			return
		}
		strValue = strconv.Itoa(*v)
	default:
		t.Fatalf("unsupported value type for setInputEnv: %T", value)
	}
	t.Setenv(inputNameAsEnv(inputName), strValue)
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
