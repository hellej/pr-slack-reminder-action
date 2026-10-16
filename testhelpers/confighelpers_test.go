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

func TestSetTestEnvironmentAppliesCollapsePRsFromAuthors(t *testing.T) {
	testConfig := GetDefaultConfigFull()
	testConfig.Config.ContentInputs.CollapsePRsFromAuthors = []string{"dependabot[bot]", "renovate[bot]"}

	SetTestEnvironment(t, testConfig, nil)

	parsedConfig, err := config.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig() returned error: %v", err)
	}
	if !slices.Equal(parsedConfig.ContentInputs.CollapsePRsFromAuthors, []string{"dependabot[bot]", "renovate[bot]"}) {
		t.Errorf("Expected both logins in CollapsePRsFromAuthors, got %q", parsedConfig.ContentInputs.CollapsePRsFromAuthors)
	}
}

func TestSetTestEnvironmentShowsMergedPRsInMessageUnlessOverridden(t *testing.T) {
	testCases := []struct {
		name      string
		overrides *map[string]any
		expected  bool
	}{
		{name: "no override", overrides: nil, expected: true},
		{
			name:      "overridden to false",
			overrides: &map[string]any{config.InputShowMergedPRsInMessage: false},
			expected:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			SetTestEnvironment(t, GetDefaultConfigFull(), tc.overrides)

			parsedConfig, err := config.GetConfig()
			if err != nil {
				t.Fatalf("GetConfig() returned error: %v", err)
			}
			if parsedConfig.ContentInputs.ShowMergedPRsInMessage != tc.expected {
				t.Errorf("Expected ShowMergedPRsInMessage %v, got %v", tc.expected, parsedConfig.ContentInputs.ShowMergedPRsInMessage)
			}
		})
	}
}
