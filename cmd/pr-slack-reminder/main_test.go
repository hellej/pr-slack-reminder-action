package main_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v72/github"
	main "github.com/hellej/pr-slack-reminder-action/cmd/pr-slack-reminder"
	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/testhelpers"
	"github.com/hellej/pr-slack-reminder-action/testhelpers/mockgithubclient"
	"github.com/hellej/pr-slack-reminder-action/testhelpers/mockslackclient"
)

type TestPRs struct {
	PRs []*github.PullRequest
	PR1 *github.PullRequest
	PR2 *github.PullRequest
	PR3 *github.PullRequest
	PR4 *github.PullRequest
	PR5 *github.PullRequest
}

func getTestPRs() TestPRs {
	now := time.Now()
	pr1 := &github.PullRequest{
		Number:    github.Ptr(1),
		CreatedAt: &github.Timestamp{Time: now.Add(-5 * time.Minute)},
		Title:     github.Ptr("This is a test PR"),
		User: &github.User{
			Login: github.Ptr("stitch"),
			Name:  github.Ptr("Stitch"),
		},
	}
	pr2 := &github.PullRequest{
		Number:    github.Ptr(2),
		CreatedAt: &github.Timestamp{Time: now.Add(-3 * time.Hour)},
		Title:     github.Ptr("This PR was created 3 hours ago and contains important changes"),
		User: &github.User{
			Login: github.Ptr("alice"),
			Name:  github.Ptr("Alice"),
		},
	}
	pr3 := &github.PullRequest{
		Number:    github.Ptr(2),
		CreatedAt: &github.Timestamp{Time: now.Add(-3 * time.Hour)},
		Title:     github.Ptr("This PR has the same time as PR2 but a longer title"),
		User: &github.User{
			Login: github.Ptr("alice"),
			Name:  github.Ptr("Alice"),
		},
	}
	pr4 := &github.PullRequest{
		Number:    github.Ptr(3),
		CreatedAt: &github.Timestamp{Time: now.Add(-26 * time.Hour)},
		Title:     github.Ptr("This PR is getting old and needs attention"),
		User: &github.User{
			Login: github.Ptr("bob"),
		},
	}
	pr5 := &github.PullRequest{
		Number:    github.Ptr(3),
		CreatedAt: &github.Timestamp{Time: now.Add(-48 * time.Hour)},
		Title:     github.Ptr("This is a big PR that no one dares to review"),
		User: &github.User{
			Name: github.Ptr("Jim"),
		},
	}

	return TestPRs{
		PRs: []*github.PullRequest{pr1, pr2, pr3, pr4, pr5},
		PR1: pr1,
		PR2: pr2,
		PR3: pr3,
		PR4: pr4,
		PR5: pr5,
	}
}

func TestScenarios(t *testing.T) {
	testCases := []struct {
		name               string
		config             testhelpers.TestConfig
		configOverrides    *map[string]any
		fetchPRsStatus     int
		fetchPRsError      error
		prs                []*github.PullRequest
		foundSlackChannels []*mockslackclient.SlackChannel
		findChannelError   error
		sendMessageError   error
		expectedErrorMsg   *string
		expectedSummary    *string
	}{
		{
			name:   "unset required inputs",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputSlackBotToken: nil,
			},
			expectedErrorMsg: testhelpers.AsPointer("configuration error: required input slack-bot-token is not set"),
		},
		{
			name:   "missing Slack inputs",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputSlackChannelID:   "",
				config.InputSlackChannelName: "",
			},
			expectedErrorMsg: testhelpers.AsPointer("configuration error: either slack-channel-id or slack-channel-name must be set"),
		},
		{
			name:   "old PRs threshold hours but no heading",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputOldPRThresholdHours: 10,
				config.InputOldPRsListHeading:   nil,
			},
			expectedErrorMsg: testhelpers.AsPointer("configuration error: if old-pr-threshold-hours is set, old-prs-list-heading must also be set"),
		},
		{
			name:             "invalid repository input",
			config:           testhelpers.GetDefaultConfigMinimal(),
			configOverrides:  &map[string]any{config.EnvGithubRepository: "invalid/repo/name"},
			expectedErrorMsg: testhelpers.AsPointer("unable to parse repository input: invalid owner/repository format: invalid/repo/name"),
		},
		{
			name:            "no PRs found with message",
			config:          testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{config.InputNoPRsMessage: "No PRs found, happy coding! 🎉"},
			expectedSummary: testhelpers.AsPointer("No PRs found, happy coding! 🎉"),
		},
		{
			name:            "no PRs found without message",
			config:          testhelpers.GetDefaultConfigMinimal(),
			expectedSummary: nil, // no message should be sent
		},
		{
			name:             "repo not found",
			config:           testhelpers.GetDefaultConfigMinimal(),
			fetchPRsStatus:   404,
			fetchPRsError:    errors.New("repository not found"),
			expectedErrorMsg: testhelpers.AsPointer("repository test-org/test-repo not found - check the repository name and permissions"),
		},
		{
			name:             "unable to fetch PRs",
			config:           testhelpers.GetDefaultConfigMinimal(),
			fetchPRsStatus:   500,
			fetchPRsError:    errors.New("unable to fetch PRs"),
			expectedErrorMsg: testhelpers.AsPointer("error fetching pull requests from test-org/test-repo: unable to fetch PRs"),
		},
		{
			name:            "no Slack channel found",
			config:          testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{config.InputNoPRsMessage: "No PRs found, happy coding! 🎉"},
			foundSlackChannels: []*mockslackclient.SlackChannel{
				{
					ID:   "C32345678",
					Name: "not-the-channel-name-provided-in-input",
				},
			},
			expectedErrorMsg: testhelpers.AsPointer("error getting channel ID by name: channel not found"),
		},
		{
			name:             "unable to fetch Slack channel(s)",
			config:           testhelpers.GetDefaultConfigMinimal(),
			findChannelError: errors.New("unable to get channels"),
			expectedErrorMsg: testhelpers.AsPointer("error getting channel ID by name: unable to get channels (check permissions and token)"),
		},
		{
			name:             "unable to send Slack message",
			config:           testhelpers.GetDefaultConfigMinimal(),
			prs:              getTestPRs().PRs,
			sendMessageError: errors.New("error in sending Slack message"),
			expectedErrorMsg: testhelpers.AsPointer("failed to send Slack message: error in sending Slack message"),
		},
		{
			name:            "minimal config with 5 PRs",
			config:          testhelpers.GetDefaultConfigMinimal(),
			prs:             getTestPRs().PRs,
			expectedSummary: testhelpers.AsPointer("5 open PRs are waiting for attention 👀"),
		},
		{
			name:            "full config with 5 PRs including old PRs",
			config:          testhelpers.GetDefaultConfigFull(),
			configOverrides: &map[string]any{config.InputOldPRThresholdHours: 12},
			prs:             getTestPRs().PRs,
			expectedSummary: testhelpers.AsPointer("5 open PRs are waiting for attention 👀"),
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			testhelpers.SetTestEnvironment(t, tc.config, tc.configOverrides)
			if tc.fetchPRsStatus == 0 {
				tc.fetchPRsStatus = 200
			}
			getGitHubClient := mockgithubclient.MakeMockGitHubClientGetter(tc.prs, tc.fetchPRsStatus, tc.fetchPRsError)
			mockSlackAPI := mockslackclient.GetMockSlackAPI(tc.foundSlackChannels, tc.findChannelError, tc.sendMessageError)
			getSlackClient := mockslackclient.MakeSlackClientGetter(mockSlackAPI)
			err := main.Run(getGitHubClient, getSlackClient)

			if tc.expectedErrorMsg == nil && err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}
			if tc.expectedErrorMsg != nil && err == nil {
				t.Errorf("Expected error: %v, got no error", *tc.expectedErrorMsg)
			}
			if tc.expectedErrorMsg != nil && err != nil && !strings.Contains(err.Error(), *tc.expectedErrorMsg) {
				t.Errorf(
					"Expected error message '%v', got: %v",
					*tc.expectedErrorMsg,
					err.Error(),
				)
			}
			if tc.expectedSummary == nil && mockSlackAPI.SentMessage.Text != "" {
				t.Errorf("Expected no summary message, but got: %v", mockSlackAPI.SentMessage.Text)
			}
			if tc.expectedSummary != nil && mockSlackAPI.SentMessage.Text != *tc.expectedSummary {
				t.Errorf(
					"Expected summary to be %v, but got: %v",
					*tc.expectedSummary,
					mockSlackAPI.SentMessage.Text,
				)
			}
		})
	}
}
