package main_test

import (
	"cmp"
	"errors"
	"slices"
	"strconv"
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

type GetTestPROptions struct {
	Number      int
	Title       string
	AuthorLogin string
	AuthorName  string
	Labels      []string
	AgeHours    float32
}

var now = time.Now()

func getTestPR(options GetTestPROptions) *github.PullRequest {
	number := cmp.Or(options.Number, testhelpers.RandomPositiveInt())
	title := cmp.Or(options.Title, testhelpers.RandomString(10))
	authorLogin := cmp.Or(options.AuthorLogin, testhelpers.RandomString(10))
	authorName := cmp.Or(options.AuthorName, strings.ToTitle(authorLogin))

	var githubLabels []*github.Label
	if len(options.Labels) == 0 {
		options.Labels = []string{testhelpers.RandomString(10)}
	}
	for _, label := range options.Labels {
		githubLabels = append(githubLabels, &github.Label{
			Name: &label,
		})
	}
	prTime := now.Add(-time.Duration(
		cmp.Or(options.AgeHours, float32(5.0))) * time.Hour,
	)

	return &github.PullRequest{
		Number: &number,
		Title:  &title,
		User: &github.User{
			Login: &authorLogin,
			Name:  &authorName,
		},
		Labels:    githubLabels,
		CreatedAt: &github.Timestamp{Time: prTime},
	}
}

type GetTestPRsOptions struct {
	Labels     []string
	AuthorUser string
}

type TestPRs struct {
	PRNumbers []int
	PRs       []*github.PullRequest
	PR1       *github.PullRequest
	PR2       *github.PullRequest
	PR3       *github.PullRequest
	PR4       *github.PullRequest
	PR5       *github.PullRequest
}

func getTestPRs(options GetTestPRsOptions) TestPRs {
	pr1 := getTestPR(GetTestPROptions{
		Number:      1,
		Title:       "This is a test PR",
		AuthorLogin: cmp.Or(options.AuthorUser, "stitch"),
		AuthorName:  cmp.Or(options.AuthorUser, "Stitch"),
		Labels:      options.Labels,
		AgeHours:    0.083, // 5 minutes
	})
	pr2 := getTestPR(GetTestPROptions{
		Number:      2,
		Title:       "This PR was created 3 hours ago and contains important changes",
		AuthorLogin: cmp.Or(options.AuthorUser, "alice"),
		AuthorName:  cmp.Or(options.AuthorUser, "Alice"),
		Labels:      options.Labels,
		AgeHours:    3,
	})
	pr3 := getTestPR(GetTestPROptions{
		Number:      3,
		Title:       "This PR has the same time as PR2 but a longer title",
		AuthorLogin: cmp.Or(options.AuthorUser, "alice"),
		AuthorName:  cmp.Or(options.AuthorUser, "Alice"),
		Labels:      options.Labels,
		AgeHours:    3,
	})
	pr4 := getTestPR(GetTestPROptions{
		Number:      4,
		Title:       "This PR is getting old and needs attention",
		AuthorLogin: cmp.Or(options.AuthorUser, "bob"),
		Labels:      options.Labels,
		AgeHours:    26,
	})
	pr5 := getTestPR(GetTestPROptions{
		Number:      5,
		Title:       "This is a big PR that no one dares to review",
		AuthorLogin: cmp.Or(options.AuthorUser, ""), // to cover the case where username is not set
		AuthorName:  cmp.Or(options.AuthorUser, "Jim"),
		Labels:      options.Labels,
		AgeHours:    48,
	})

	return TestPRs{
		PRNumbers: []int{1, 2, 3, 4, 5},
		PRs:       []*github.PullRequest{pr1, pr2, pr3, pr4, pr5},
		PR1:       pr1,
		PR2:       pr2,
		PR3:       pr3,
		PR4:       pr4,
		PR5:       pr5,
	}
}

func filterPRsByNumbers(
	prs []*github.PullRequest,
	prsByRepo map[string][]*github.PullRequest,
	numbers []int,
) []*github.PullRequest {
	var filteredPRs []*github.PullRequest
	for _, pr := range prs {
		if slices.Contains(numbers, *pr.Number) {
			filteredPRs = append(filteredPRs, pr)
		}
	}
	for _, prList := range prsByRepo {
		for _, pr := range prList {
			if slices.Contains(numbers, *pr.Number) {
				filteredPRs = append(filteredPRs, pr)
			}
		}
	}
	return filteredPRs
}

func TestScenarios(t *testing.T) {
	testCases := []struct {
		name               string
		config             testhelpers.TestConfig
		configOverrides    *map[string]any
		fetchPRsStatus     int
		fetchPRsError      error
		prs                []*github.PullRequest
		prsByRepo          map[string][]*github.PullRequest
		reviewsByPRNumber  map[int][]*github.PullRequestReview
		foundSlackChannels []*mockslackclient.SlackChannel
		findChannelError   error
		sendMessageError   error
		expectedErrorMsg   string
		expectedPRNumbers  []int
		expectedSummary    string
	}{
		{
			name:   "unset required inputs",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputSlackBotToken: nil,
			},
			expectedErrorMsg: "configuration error: required input slack-bot-token is not set",
		},
		{
			name:   "missing Slack inputs",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputSlackChannelID:   "",
				config.InputSlackChannelName: "",
			},
			expectedErrorMsg: "configuration error: either slack-channel-id or slack-channel-name must be set",
		},
		{
			name:   "old PRs threshold hours but no heading",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputOldPRThresholdHours: 10,
				config.InputOldPRsListHeading:   nil,
			},
			expectedErrorMsg: "configuration error: if old-pr-threshold-hours is set, old-prs-list-heading must also be set",
		},
		{
			name:             "invalid repository input 1",
			config:           testhelpers.GetDefaultConfigMinimal(),
			configOverrides:  &map[string]any{config.EnvGithubRepository: "invalid/repo/name"},
			expectedErrorMsg: "configuration error: invalid repositories input: invalid owner/repository format: invalid/repo/name",
		},
		{
			name:             "invalid repository input 2",
			config:           testhelpers.GetDefaultConfigMinimal(),
			configOverrides:  &map[string]any{config.EnvGithubRepository: "invalid/"},
			expectedErrorMsg: "configuration error: invalid repositories input: owner or repository name cannot be empty in: invalid/",
		},
		{
			name:            "no PRs found with message",
			config:          testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{config.InputNoPRsMessage: "No PRs found, happy coding! 🎉"},
			expectedSummary: "No PRs found, happy coding! 🎉",
		},
		{
			name:             "invalid global filters input 1",
			config:           testhelpers.GetDefaultConfigMinimal(),
			configOverrides:  &map[string]any{config.InputGlobalFilters: "{\"invalid\": \"json\"}"},
			expectedErrorMsg: "configuration error: error reading input filters: unable to parse filters from {\"invalid\": \"json\"}: json: unknown field \"invalid\"",
		},
		{
			name:             "invalid global filters input 2",
			config:           testhelpers.GetDefaultConfigMinimal(),
			configOverrides:  &map[string]any{config.InputGlobalFilters: "{\"authors\": [\"alice\"], \"authors-ignore\": [\"bob\"]}"},
			expectedErrorMsg: "configuration error: error reading input filters: invalid filters: {\"authors\": [\"alice\"], \"authors-ignore\": [\"bob\"]}, error: cannot use both authors and authors-ignore filters at the same time",
		},
		{
			name:            "no PRs found without message",
			config:          testhelpers.GetDefaultConfigMinimal(),
			expectedSummary: "", // no message should be sent
		},
		{
			name:             "repo not found",
			config:           testhelpers.GetDefaultConfigMinimal(),
			fetchPRsStatus:   404,
			fetchPRsError:    errors.New("repository not found"),
			expectedErrorMsg: "repository test-org/test-repo not found - check the repository name and permissions",
		},
		{
			name:             "unable to fetch PRs",
			config:           testhelpers.GetDefaultConfigMinimal(),
			fetchPRsStatus:   500,
			fetchPRsError:    errors.New("unable to fetch PRs"),
			expectedErrorMsg: "error fetching pull requests from test-org/test-repo: unable to fetch PRs",
		},
		{
			name:   "no Slack channel found",
			config: testhelpers.GetDefaultConfigMinimal(),
			foundSlackChannels: []*mockslackclient.SlackChannel{
				{
					ID:   "C32345678",
					Name: "not-the-channel-name-provided-in-input",
				},
			},
			expectedErrorMsg: "error getting channel ID by name: channel not found",
		},
		{
			name:             "unable to fetch Slack channel(s)",
			config:           testhelpers.GetDefaultConfigMinimal(),
			findChannelError: errors.New("unable to get channels"),
			expectedErrorMsg: "error getting channel ID by name: unable to get channels (check permissions and token)",
		},
		{
			name:             "unable to send Slack message",
			config:           testhelpers.GetDefaultConfigMinimal(),
			prs:              getTestPRs(GetTestPRsOptions{}).PRs,
			sendMessageError: errors.New("error in sending Slack message"),
			expectedErrorMsg: "failed to send Slack message: error in sending Slack message",
		},
		{
			name:              "minimal config with 5 PRs",
			config:            testhelpers.GetDefaultConfigMinimal(),
			prs:               getTestPRs(GetTestPRsOptions{}).PRs,
			expectedPRNumbers: getTestPRs(GetTestPRsOptions{}).PRNumbers,
			expectedSummary:   "5 open PRs are waiting for attention 👀",
		},
		{
			name:            "all PRs filtered out by labels (by inclusion)",
			config:          testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{config.InputGlobalFilters: "{\"labels\": [\"infra\"]}"},
			prs:             getTestPRs(GetTestPRsOptions{}).PRs,
			expectedSummary: "", // no message should be sent
		},
		{
			name:            "all PRs filtered out by labels (by exclusion)",
			config:          testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{config.InputGlobalFilters: "{\"labels-ignore\": [\"label-to-ignore\"]}"},
			prs:             getTestPRs(GetTestPRsOptions{Labels: []string{"label-to-ignore"}}).PRs,
			expectedSummary: "", // no message should be sent
		},
		{
			name:            "PRs by one user filtered out",
			config:          testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{config.InputGlobalFilters: "{\"authors-ignore\": [\"alice\"]}"},
			prs: []*github.PullRequest{
				getTestPR(GetTestPROptions{Number: 1, AuthorLogin: "alice", Title: "PR by Alice"}),
				getTestPR(GetTestPROptions{Number: 2, AuthorLogin: "bob", Title: "PR by Bob"}),
			},
			expectedPRNumbers: []int{2},
			expectedSummary:   "1 open PR is waiting for attention 👀",
		},
		{
			name:            "all PRs filtered out by users (by inclusion)",
			config:          testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{config.InputGlobalFilters: "{\"authors\": [\"lilo\"]}"},
			prs:             getTestPRs(GetTestPRsOptions{}).PRs,
			expectedSummary: "", // no message should be sent
		},
		{
			name:            "all PRs filtered out by users (by exclusion)",
			config:          testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{config.InputGlobalFilters: "{\"authors-ignore\": [\"lilo\"]}"},
			prs:             getTestPRs(GetTestPRsOptions{AuthorUser: "lilo"}).PRs,
			expectedSummary: "", // no message should be sent
		},
		{
			name:   "PRs by user in one repo filtered",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputRepositoryFilters:  "repo1: {\"authors-ignore\": [\"alice\"]}",
				config.InputGithubRepositories: "some-org/repo1; some-org/repo2",
			},
			prsByRepo: map[string][]*github.PullRequest{
				"repo1": {
					getTestPR(GetTestPROptions{Number: 1, AuthorLogin: "alice", Title: "The PR by Alice that should be excluded"}),
				},
				"repo2": {
					getTestPR(GetTestPROptions{Number: 2, AuthorLogin: "alice", Title: "PR by Alice that should be included"}),
				},
			},
			expectedPRNumbers: []int{2},
			expectedSummary:   "1 open PR is waiting for attention 👀",
		},
		{
			name:   "full config with 5 PRs including old PRs",
			config: testhelpers.GetDefaultConfigFull(),
			configOverrides: &map[string]any{
				config.InputOldPRThresholdHours: 12,
				config.InputGlobalFilters:       "{\"labels\": [\"feature\", \"fix\"]}",
			},
			prs:               getTestPRs(GetTestPRsOptions{Labels: []string{"feature"}}).PRs,
			expectedPRNumbers: getTestPRs(GetTestPRsOptions{}).PRNumbers,
			reviewsByPRNumber: map[int][]*github.PullRequestReview{
				*getTestPRs(GetTestPRsOptions{}).PR1.Number: {
					{
						ID:    github.Ptr(int64(1)),
						Body:  github.Ptr("LGTM ✅"),
						User:  &github.User{Login: github.Ptr("reviewer1")},
						State: github.Ptr("APPROVED"),
					},
				},
				*getTestPRs(GetTestPRsOptions{}).PR2.Number: {
					{
						ID:    github.Ptr(int64(2)),
						Body:  github.Ptr("LGTM, just a few comments..."),
						User:  &github.User{Login: github.Ptr("reviewer2")},
						State: github.Ptr("COMMENTED"),
					},
				},
			},
			expectedSummary: "5 open PRs are waiting for attention 👀",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testhelpers.SetTestEnvironment(t, tc.config, tc.configOverrides)

			getGitHubClient := mockgithubclient.MakeMockGitHubClientGetter(
				tc.prs, tc.prsByRepo, cmp.Or(tc.fetchPRsStatus, 200), tc.fetchPRsError, tc.reviewsByPRNumber,
			)
			mockSlackAPI := mockslackclient.GetMockSlackAPI(tc.foundSlackChannels, tc.findChannelError, tc.sendMessageError)
			getSlackClient := mockslackclient.MakeSlackClientGetter(mockSlackAPI)
			err := main.Run(getGitHubClient, getSlackClient)

			if tc.expectedErrorMsg == "" && err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}
			if tc.expectedErrorMsg != "" && err == nil {
				t.Errorf("Expected error: %v, got no error", tc.expectedErrorMsg)
			}
			if tc.expectedErrorMsg != "" && err != nil && !strings.Contains(err.Error(), tc.expectedErrorMsg) {
				t.Errorf(
					"Expected error message '%v', got: %v",
					tc.expectedErrorMsg,
					err.Error(),
				)
			}
			if tc.expectedSummary == "" && mockSlackAPI.SentMessage.Text != "" {
				t.Errorf("Expected no summary message, but got: %v", mockSlackAPI.SentMessage.Text)
			}
			if tc.expectedSummary != "" && mockSlackAPI.SentMessage.Text != tc.expectedSummary {
				t.Errorf(
					"Expected summary to be %v, but got: %v",
					tc.expectedSummary,
					mockSlackAPI.SentMessage.Text,
				)
			}
			if tc.expectedErrorMsg != "" {
				return
			}
			expectedPRs := filterPRsByNumbers(tc.prs, tc.prsByRepo, tc.expectedPRNumbers)
			if len(expectedPRs) != len(tc.expectedPRNumbers) {
				t.Errorf("Test config error: test PRs do not contain all PRs by expectedPRNumbers")
			}
			if len(expectedPRs) > 0 {
				for _, pr := range expectedPRs {
					if !mockSlackAPI.SentMessage.Blocks.ContainsPRTitle(*pr.Title) {
						t.Errorf("Expected PR title '%s' to be in the sent message blocks", *pr.Title)
					}
				}
			}
			if len(expectedPRs) != mockSlackAPI.SentMessage.Blocks.GetPRCount() {
				t.Errorf(
					"Expected %v PRs to be included in the message (was %v)",
					len(expectedPRs), mockSlackAPI.SentMessage.Blocks.GetPRCount(),
				)
			}
			expectedHeading := ""
			if len(expectedPRs) > 0 {
				expectedHeading = strings.ReplaceAll(
					tc.config.ContentInputs.MainListHeading, "<pr_count>", strconv.Itoa(len(expectedPRs)),
				)
			}
			if expectedHeading != "" && !mockSlackAPI.SentMessage.Blocks.ContainsHeading(expectedHeading) {
				t.Errorf(
					"Expected PR list heading '%s' to be included in the Slack message", expectedHeading,
				)
			}
		})
	}
}
