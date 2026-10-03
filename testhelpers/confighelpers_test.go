package testhelpers

import (
	"slices"
	"testing"

	"github.com/hellej/pr-slack-reminder-action/internal/config"
)

func TestSetTestEnvironmentAppliesGroupByRepository(t *testing.T) {
	testConfig := GetDefaultConfigFull()
	testConfig.Config.ContentInputs.GroupByRepository = true

	SetTestEnvironment(t, testConfig, nil)

	parsedConfig, err := config.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig() returned error: %v", err)
	}
	if !parsedConfig.ContentInputs.GroupByRepository {
		t.Error("Expected GroupByRepository to be true")
	}
}

func TestSetTestEnvironmentAppliesCollapsedPRAuthors(t *testing.T) {
	testConfig := GetDefaultConfigFull()
	testConfig.Config.ContentInputs.CollapsedPRAuthors = []string{"dependabot[bot]", "renovate[bot]"}

	SetTestEnvironment(t, testConfig, nil)

	parsedConfig, err := config.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig() returned error: %v", err)
	}
	if !slices.Equal(parsedConfig.ContentInputs.CollapsedPRAuthors, []string{"dependabot[bot]", "renovate[bot]"}) {
		t.Errorf("Expected both collapsed PR authors, got %q", parsedConfig.ContentInputs.CollapsedPRAuthors)
	}
}
