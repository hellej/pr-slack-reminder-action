package testhelpers

import (
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
