package main_test

import (
	"cmp"
	"errors"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/google/go-github/v78/github"
	main "github.com/hellej/pr-slack-reminder-action/cmd/pr-slack-reminder"
	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/state"
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
	Draft       *bool  // nil means unset, github.Ptr(true) means draft, github.Ptr(false) means not draft
	State       string // "open", "closed"
	Merged      bool   // true if PR is merged
}

var now = time.Now()

func getTestPR(options GetTestPROptions) *github.PullRequest {
	number := cmp.Or(options.Number, testhelpers.RandomPositiveInt())
	title := cmp.Or(options.Title, testhelpers.RandomString(10))
	authorLogin := cmp.Or(options.AuthorLogin, testhelpers.RandomString(10))
	authorName := cmp.Or(options.AuthorName, cases.Title(language.English).String(authorLogin))

	var githubLabels []*github.Label
	if len(options.Labels) == 0 {
		options.Labels = []string{testhelpers.RandomString(10)}
	}
	for _, label := range options.Labels {
		githubLabels = append(githubLabels, &github.Label{
			Name: &label,
		})
	}

	ageMinutes := int(
		math.Round(
			float64(cmp.Or(options.AgeHours, float32(5.0)) * 60),
		),
	)
	prTime := now.Add(-time.Duration(ageMinutes) * time.Minute)

	state := cmp.Or(options.State, "open")

	return &github.PullRequest{
		Number: &number,
		Title:  &title,
		User: &github.User{
			Login: &authorLogin,
			Name:  &authorName,
		},
		Labels:    githubLabels,
		CreatedAt: &github.Timestamp{Time: prTime},
		Draft:     options.Draft,
		State:     &state,
		Merged:    &options.Merged,
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

type GetTestStateOptions struct {
	PRNumbers []int
}

func getTestState(options GetTestStateOptions) state.State {
	prRefs := make([]models.PullRequestRef, 0, len(options.PRNumbers))
	for _, prNumber := range options.PRNumbers {
		prRefs = append(prRefs, models.PullRequestRef{
			Repository: models.Repository{
				Owner: "test-org",
				Name:  "test-repo",
			},
			Number: prNumber,
		})
	}

	return state.State{
		SchemaVersion: 1,
		CreatedAt:     time.Now().Add(-1 * time.Hour),
		SlackMessage: state.SlackRef{
			ChannelID: "C12345678",
			MessageTS: "1623850245.000200",
		},
		PullRequests: prRefs,
	}
}

func TestScenarios(t *testing.T) {
	testCases := []struct {
		name                string
		config              testhelpers.TestConfig
		configOverrides     *map[string]any
		fetchPRsStatus      int
		prServiceError      error
		issueServiceError   error
		prs                 []*github.PullRequest
		prsByRepo           map[string][]*github.PullRequest
		reviewsByPRNumber   map[int][]*github.PullRequestReview
		foundSlackChannels  []*mockslackclient.SlackChannel
		findChannelError    error
		sendMessageError    error
		expectedErrorMsg    string
		expectedPRNumbers   []int
		expectedPRItemTexts []string
		expectedSummary     string
		expectedHeadings    []string // For group-by-repository mode to check repository headings
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
			name:             "invalid repository input 1",
			config:           testhelpers.GetDefaultConfigMinimal(),
			configOverrides:  &map[string]any{config.InputGithubRepositories: []string{"invalid/repo/name"}},
			expectedErrorMsg: "configuration error: invalid repositories input: invalid owner/repository format: invalid/repo/name",
		},
		{
			name:             "invalid repository input 2",
			config:           testhelpers.GetDefaultConfigMinimal(),
			configOverrides:  &map[string]any{config.InputGithubRepositories: []string{"invalid/"}},
			expectedErrorMsg: "configuration error: invalid repositories input: owner or repository name cannot be empty in: invalid/",
		},
		{
			name:   "too many repositories",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputGithubRepositories: func() string {
					var repos []string
					for i := 1; i <= 31; i++ {
						repos = append(repos, "org"+strconv.Itoa(i)+"/repo"+strconv.Itoa(i))
					}
					return strings.Join(repos, "\n")
				}(),
			},
			expectedErrorMsg: "configuration error: too many repositories: maximum of 30 repositories allowed, got 31",
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
			name:             "invalid global filters input: conflicting labels and labels-ignore",
			config:           testhelpers.GetDefaultConfigMinimal(),
			configOverrides:  &map[string]any{config.InputGlobalFilters: "{\"labels\": [\"infra\"], \"labels-ignore\": [\"infra\"]}"},
			expectedErrorMsg: "configuration error: error reading input filters: invalid filters: {\"labels\": [\"infra\"], \"labels-ignore\": [\"infra\"]}, error: labels filter cannot contain labels that are in labels-ignore filter",
		},
		{
			name:             "invalid repository filters input: invalid mapping",
			config:           testhelpers.GetDefaultConfigMinimal(),
			configOverrides:  &map[string]any{config.InputRepositoryFilters: "asdf"},
			expectedErrorMsg: "configuration error: error reading input repository-filters: invalid mapping format for repository-filters: 'asdf'",
		},
		{
			name:             "invalid repository filters input: conflicting labels and labels-ignore",
			config:           testhelpers.GetDefaultConfigMinimal(),
			configOverrides:  &map[string]any{config.InputRepositoryFilters: "\"test-repo\": {\"labels\": [\"infra\"], \"labels-ignore\": [\"infra\"]}"},
			expectedErrorMsg: "configuration error: error parsing filters for repository \"test-repo\": invalid filters: {\"labels\": [\"infra\"], \"labels-ignore\": [\"infra\"]}, error: labels filter cannot contain labels that are in labels-ignore filter",
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
			prServiceError:   errors.New("repository not found"),
			expectedErrorMsg: "repository test-org/test-repo not found - check the repository name and permissions",
		},
		{
			name:             "unable to fetch PRs",
			config:           testhelpers.GetDefaultConfigMinimal(),
			fetchPRsStatus:   500,
			prServiceError:   errors.New("unable to fetch PRs"),
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
			expectedErrorMsg: "error getting channel ID by name: unable to get channels, unable to get channels (unable to fetch channels, check token and permissions or use channel ID input instead)",
		},
		{
			name:             "unable to send Slack message",
			config:           testhelpers.GetDefaultConfigMinimal(),
			prs:              getTestPRs(GetTestPRsOptions{}).PRs,
			sendMessageError: errors.New("error in sending Slack message"),
			expectedErrorMsg: "failed to send Slack message: error in sending Slack message",
		},
		{
			name:              "timeline comments fetch error is handled gracefully",
			config:            testhelpers.GetDefaultConfigMinimal(),
			prs:               getTestPRs(GetTestPRsOptions{}).PRs,
			issueServiceError: errors.New("error fetching timeline comments"),
			expectedPRNumbers: getTestPRs(GetTestPRsOptions{}).PRNumbers,
			expectedSummary:   "5 open PRs are waiting for attention 👀",
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
			name:   "draft PRs are filtered out",
			config: testhelpers.GetDefaultConfigMinimal(),
			prs: []*github.PullRequest{
				getTestPR(GetTestPROptions{Number: 1, Title: "Regular PR", AuthorLogin: "alice", Draft: github.Ptr(false)}),
				getTestPR(GetTestPROptions{Number: 2, Title: "Draft PR", AuthorLogin: "bob", Draft: github.Ptr(true)}),
				getTestPR(GetTestPROptions{Number: 3, Title: "Unset draft PR", AuthorLogin: "charlie", Draft: nil}),
			},
			expectedPRNumbers: []int{1, 3}, // draft PR should be excluded, nil should be included
			expectedSummary:   "2 open PRs are waiting for attention 👀",
		},
		{
			name:   "all PRs filtered out when all are drafts",
			config: testhelpers.GetDefaultConfigMinimal(),
			prs: []*github.PullRequest{
				getTestPR(GetTestPROptions{Number: 1, Title: "Draft PR 1", AuthorLogin: "alice", Draft: github.Ptr(true)}),
				getTestPR(GetTestPROptions{Number: 2, Title: "Draft PR 2", AuthorLogin: "bob", Draft: github.Ptr(true)}),
			},
			expectedSummary: "", // no message should be sent since all PRs are drafts
		},
		{
			name:   "PRs by user in one repo filtered",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputGithubRepositories: "some-org/repo1; some-org/repo2",
				config.InputRepositoryFilters:  "repo1: {\"authors-ignore\": [\"alice\"]}",
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
			name:   "PRs by user in one repo filtered by repository filter using full owner/repo reference",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputGithubRepositories: "some-org/repo1; some-org/repo2",
				config.InputRepositoryFilters:  "some-org/repo1: {\"authors-ignore\": [\"alice\"]}",
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
			name:   "PRs not filtered out from repo2 by overriding global filters with empty repository filters for repo2",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputGithubRepositories: "some-org/repo1; some-org/repo2",
				config.InputGlobalFilters:      "{\"authors-ignore\": [\"alice\"]}",
				config.InputRepositoryFilters:  "repo2: {}",
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
				config.InputPRLinkRepoPrefixes:  "test-repo: 'TR / '",
			},
			prs:               getTestPRs(GetTestPRsOptions{Labels: []string{"feature"}}).PRs,
			expectedPRNumbers: getTestPRs(GetTestPRsOptions{}).PRNumbers,
			expectedPRItemTexts: []string{
				"TR / This is a test PR 5 minutes ago by Stitch",
				"TR / This PR was created 3 hours ago and contains important changes 3 hours ago by U2234567890",
				"TR / This PR has the same time as PR2 but a longer title 3 hours ago by U2234567890",
				"TR / This PR is getting old and needs attention 🚨 1 days old by U3234567890",
				"TR / This is a big PR that no one dares to review 🚨 2 days old by Jim",
			},
			expectedSummary: "5 open PRs are waiting for attention 👀",
		},
		{
			name:   "old PR highlighting with alarm emojis",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputOldPRThresholdHours: 24,
			},
			prs: []*github.PullRequest{
				getTestPR(GetTestPROptions{
					Number:      1,
					Title:       "Recent PR",
					AuthorLogin: "alice",
					AgeHours:    2,
				}),
				getTestPR(GetTestPROptions{
					Number:      2,
					Title:       "Old PR needs attention",
					AuthorLogin: "bob",
					AgeHours:    48,
				}),
			},
			expectedPRNumbers: []int{1, 2},
			expectedPRItemTexts: []string{
				"Recent PR 2 hours ago by Alice",
				"Old PR needs attention 🚨 2 days old by Bob",
			},
			expectedSummary: "2 open PRs are waiting for attention 👀",
		},
		{
			name:   "5 PRs of which some are approved and some are commented",
			config: testhelpers.GetDefaultConfigMinimal(),
			prs: []*github.PullRequest{
				getTestPR(GetTestPROptions{
					Number:      1,
					Title:       "PR 1",
					AuthorLogin: "stitch",
					AuthorName:  "Stitch",
					AgeHours:    0.083, // 5 minutes
				}),
				getTestPR(GetTestPROptions{
					Number:      2,
					Title:       "PR 2",
					AuthorLogin: "alice",
					AuthorName:  "Alice",
					AgeHours:    3,
				}),
				getTestPR(GetTestPROptions{
					Number:      3,
					Title:       "PR 3",
					AuthorLogin: "alice",
					AuthorName:  "Alice",
					AgeHours:    48,
				}),
				getTestPR(GetTestPROptions{
					Number:      4,
					Title:       "PR 4",
					AuthorLogin: "jim",
					AuthorName:  "Jim",
					AgeHours:    5,
				}),
			},
			expectedPRNumbers: []int{1, 2, 3, 4},
			expectedPRItemTexts: []string{
				"PR 1 5 minutes ago by Stitch (✅ reviewer1, reviewer2)",
				"PR 2 3 hours ago by Alice (💬 reviewer1, reviewer2)",
				"PR 3 2 days ago by Alice (💬 reviewer3)",
				"PR 4 5 hours ago by Jim (✅ reviewer2 / 💬 reviewer3)",
			},
			reviewsByPRNumber: map[int][]*github.PullRequestReview{
				*getTestPRs(GetTestPRsOptions{}).PR1.Number: {
					mockgithubclient.NewReview(1, "APPROVED", "reviewer1", "", "LGTM 🙏🏻"),
					mockgithubclient.NewReview(2, "APPROVED", "reviewer2", "", "LGTM 🚀"),
				},
				*getTestPRs(GetTestPRsOptions{}).PR2.Number: {
					mockgithubclient.NewReview(3, "COMMENTED", "reviewer1", "", "LGTM, just a few comments..."),
					mockgithubclient.NewReview(4, "COMMENTED", "reviewer2", "", "Looks good but..."),
				},
				*getTestPRs(GetTestPRsOptions{}).PR3.Number: {
					mockgithubclient.NewReview(5, "COMMENTED", "reviewer3", "", "Splendid work! Just a few questions..."),
				},
				*getTestPRs(GetTestPRsOptions{}).PR4.Number: {
					mockgithubclient.NewReview(6, "COMMENTED", "reviewer3", "", "Splendid work! Just a few questions..."),
					mockgithubclient.NewReview(7, "COMMENTED", "reviewer3", "", "Splendid work! Just a few questions..."), // duplicate review by reviewer3 should be omitted
					mockgithubclient.NewReview(8, "APPROVED", "reviewer2", "", "LGTM 🚀"),
					mockgithubclient.NewReview(9, "APPROVED", "reviewer2", "", "LGTM again 🚀"), // duplicate approval by reviewer2 should be omitted
				},
			},
			expectedSummary: "4 open PRs are waiting for attention 👀",
		},
		{
			name:   "group by repository with single repo",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputGroupByRepository: true,
			},
			prs: []*github.PullRequest{
				getTestPR(GetTestPROptions{Number: 1, Title: "PR 1", AuthorLogin: "alice"}),
				getTestPR(GetTestPROptions{Number: 2, Title: "PR 2", AuthorLogin: "bob"}),
			},
			expectedPRNumbers: []int{1, 2},
			expectedSummary:   "2 open PRs are waiting for attention 👀",
			expectedHeadings:  []string{"Open PRs in test-org/test-repo:"},
		},
		{
			name:   "group by repository with multiple repos",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputGithubRepositories: "org/repo1; org/repo2",
				config.InputGroupByRepository:  true,
			},
			prsByRepo: map[string][]*github.PullRequest{
				"repo1": {
					getTestPR(GetTestPROptions{Number: 1, Title: "PR from repo1", AuthorLogin: "alice"}),
				},
				"repo2": {
					getTestPR(GetTestPROptions{Number: 2, Title: "PR from repo2", AuthorLogin: "bob"}),
					getTestPR(GetTestPROptions{Number: 3, Title: "Another PR from repo2", AuthorLogin: "charlie"}),
				},
			},
			expectedPRNumbers: []int{1, 2, 3},
			expectedSummary:   "3 open PRs are waiting for attention 👀",
			expectedHeadings:  []string{"Open PRs in org/repo1:", "Open PRs in org/repo2:"},
		},
		{
			name:   "group by repository disabled with main list heading required",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputGroupByRepository: false,
				config.InputPRListHeading:     "", // Empty heading when grouping is disabled should cause error
			},
			expectedErrorMsg: "configuration error: main-list-heading is required when group-by-repository is false",
		},
		{
			name:   "group by repository enabled ignores main list heading",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputGroupByRepository: true,
				config.InputPRListHeading:     "", // Empty heading should be ignored when grouping is enabled
			},
			prs: []*github.PullRequest{
				getTestPR(GetTestPROptions{Number: 1, Title: "Test PR", AuthorLogin: "alice"}),
			},
			expectedPRNumbers: []int{1},
			expectedSummary:   "1 open PR is waiting for attention 👀",
		},
		{
			name:   "reviews by bots and author are excluded from review status",
			config: testhelpers.GetDefaultConfigMinimal(),
			prs: []*github.PullRequest{
				getTestPR(GetTestPROptions{Number: 1, Title: "PR with bot and human reviewers", AuthorLogin: "alice"}),
			},
			reviewsByPRNumber: map[int][]*github.PullRequestReview{
				1: {
					mockgithubclient.NewReview(1, "COMMENTED", "human-reviewer", "Human Reviewer", "Human feedback", "User"),
					mockgithubclient.NewReview(2, "COMMENTED", "alice", "Alice", "Self review", "User"),
					mockgithubclient.NewReview(3, "COMMENTED", "dependabot", "Dependabot", "Bot feedback", "Bot"),
					mockgithubclient.NewReview(4, "APPROVED", "codecov", "Codecov", "Bot approval", "Bot"),
				},
			},
			expectedPRNumbers: []int{1},
			expectedSummary:   "1 open PR is waiting for attention 👀",
			expectedPRItemTexts: []string{
				"PR with bot and human reviewers 5 hours ago by Alice (💬 Human Reviewer)",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testhelpers.SetTestEnvironment(t, tc.config, tc.configOverrides)

			getGitHubClient := mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
				PRs:                   tc.prs,
				PRsByRepo:             tc.prsByRepo,
				ListPRsResponseStatus: cmp.Or(tc.fetchPRsStatus, 200),
				ReviewsByPRNumber:     tc.reviewsByPRNumber,
				PRServiceError:        tc.prServiceError,
				IssueServiceError:     tc.issueServiceError,
			})
			mockSlackAPI := mockslackclient.GetMockSlackAPI(tc.foundSlackChannels, tc.findChannelError, tc.sendMessageError, nil)
			getSlackClient := mockslackclient.MakeSlackClientGetter(mockSlackAPI)
			err := main.Run(getGitHubClient, getSlackClient)

			if tc.expectedErrorMsg == "" && err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}
			if tc.expectedErrorMsg != "" && err == nil {
				t.Errorf("Expected error: %v, got no error", tc.expectedErrorMsg)
			}
			if tc.expectedErrorMsg != "" && err != nil && !strings.Contains(err.Error(), tc.expectedErrorMsg) {
				t.Errorf("Expected error message '%v', got: %v", tc.expectedErrorMsg, err)
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
					if !mockSlackAPI.SentMessage.Blocks.SomePRItemContainsText(*pr.Title) {
						t.Errorf("Expected PR title '%s' to be included in the sent message blocks", *pr.Title)
					}
				}
			}
			if len(tc.expectedPRItemTexts) > 0 {
				for _, expectedText := range tc.expectedPRItemTexts {
					if !mockSlackAPI.SentMessage.Blocks.SomePRItemTextIsEqualTo(expectedText) {
						t.Errorf(
							"Expected list item text '%s' to be in the sent message blocks", expectedText,
						)
						prItems := mockSlackAPI.SentMessage.Blocks.GetAllPRItemTexts()
						t.Logf("Found PR items:")
						for _, prItem := range prItems {
							t.Log(prItem)
						}
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
			// Check if grouping is enabled in overrides
			groupByRepository := tc.config.ContentInputs.GroupByRepository
			if tc.configOverrides != nil {
				if override, exists := (*tc.configOverrides)[config.InputGroupByRepository]; exists {
					if groupBool, ok := override.(bool); ok {
						groupByRepository = groupBool
					}
				}
			}
			// Only expect main list heading when not grouping by repository
			if len(expectedPRs) > 0 && !groupByRepository {
				expectedHeading = strings.ReplaceAll(
					tc.config.ContentInputs.PRListHeading, "<pr_count>", strconv.Itoa(len(expectedPRs)),
				)
			}
			if expectedHeading != "" && !mockSlackAPI.SentMessage.Blocks.ContainsHeading(expectedHeading) {
				t.Errorf(
					"Expected PR list heading '%s' to be included in the Slack message", expectedHeading,
				)
			}
			// Check for expected repository headings (used in group-by-repository mode)
			for _, expectedHeading := range tc.expectedHeadings {
				if !mockSlackAPI.SentMessage.Blocks.ContainsHeading(expectedHeading) {
					t.Errorf(
						"Expected repository heading '%s' to be included in the Slack message", expectedHeading,
					)
					prLists := mockSlackAPI.SentMessage.Blocks.GetPRLists()
					t.Logf("Found headings:")
					for _, prList := range prLists {
						t.Logf("  - '%s'", prList.Heading)
					}
				}
			}
		})
	}
}

func TestPostModeStateSaving(t *testing.T) {
	testStateFilePath := "/tmp/test-state.json"

	postModeConfig := testhelpers.GetDefaultConfigFull()
	configOverrides := map[string]any{
		config.InputRunMode:     config.RunModePost,
		config.EnvStateFilePath: testStateFilePath,
	}
	testhelpers.SetTestEnvironment(t, postModeConfig, &configOverrides)

	testPRs := getTestPRs(GetTestPRsOptions{})

	mockGitHubClientGetter := mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
		PRs: testPRs.PRs,
	})
	mockSlackAPI := mockslackclient.GetMockSlackAPI(nil, nil, nil, nil)

	err := main.Run(
		mockGitHubClientGetter,
		mockslackclient.MakeSlackClientGetter(mockSlackAPI),
	)

	if err != nil {
		t.Fatalf("Expected Run to succeed, but got error: %v", err)
	}

	if _, err := os.Stat(testStateFilePath); os.IsNotExist(err) {
		t.Errorf("Expected state file to be created at %s, but it doesn't exist", testStateFilePath)
		return
	}

	var loadedState state.State
	err = testhelpers.LoadJSONFromFile(testStateFilePath, &loadedState)
	if err != nil {
		t.Fatalf("Failed to load state file: %v", err)
	}

	if err := loadedState.Validate(); err != nil {
		t.Errorf("State validation failed: %v", err)
	}

	expectedChannelID := "C12345678" // From mock
	if loadedState.SlackMessage.ChannelID != expectedChannelID {
		t.Errorf("Expected channel ID %s, got %s", expectedChannelID, loadedState.SlackMessage.ChannelID)
	}

	if loadedState.SlackMessage.MessageTS == "" {
		t.Error("Expected message timestamp to be set in state")
	}

	if len(loadedState.PullRequests) != len(testPRs.PRs) {
		t.Errorf("Expected %d PRs in state, got %d", len(testPRs.PRs), len(loadedState.PullRequests))
	}

	defer func() {
		if err := os.Remove(testStateFilePath); err != nil {
			t.Logf("Failed to clean up test state file: %v", err)
		}
	}()
}

func TestScenariosUpdateMode(t *testing.T) {
	testCases := []struct {
		name                   string
		config                 testhelpers.TestConfig
		configOverrides        *map[string]any
		mockState              *state.State
		prByNumber             map[int]*github.PullRequest
		fetchPRErrorByPRNumber map[int]error
		reviewsByPRNumber      map[int][]*github.PullRequestReview
		listArtifactsError     error
		downloadArtifactError  error
		updateMessageError     error
		expectedErrorMsg       string
		expectedPRItemTexts    []string
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
			name:   "update mode with empty state exits gracefully",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputRunMode: config.RunModeUpdate,
			},
			mockState: testhelpers.AsPointer(getTestState(GetTestStateOptions{PRNumbers: []int{}})),
		},
		{
			name:   "update mode with all PRs filtered out exits gracefully",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputRunMode:       config.RunModeUpdate,
				config.InputGlobalFilters: "{\"authors-ignore\": [\"alice\", \"bob\"]}",
			},
			mockState: testhelpers.AsPointer(getTestState(GetTestStateOptions{PRNumbers: []int{1, 2}})),
			prByNumber: map[int]*github.PullRequest{
				1: getTestPR(GetTestPROptions{Number: 1, Title: "First PR", AuthorLogin: "alice"}),
				2: getTestPR(GetTestPROptions{Number: 2, Title: "Second PR", AuthorLogin: "bob"}),
			},
		},
		{
			name:   "update mode with two PRs with reviewers",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputRunMode: config.RunModeUpdate,
			},
			mockState: testhelpers.AsPointer(getTestState(GetTestStateOptions{PRNumbers: []int{1, 2}})),
			prByNumber: map[int]*github.PullRequest{
				1: getTestPR(GetTestPROptions{Number: 1, Title: "First PR", AuthorLogin: "alice"}),
				2: getTestPR(GetTestPROptions{Number: 2, Title: "Second PR", AuthorLogin: "bob"}),
			},
			reviewsByPRNumber: map[int][]*github.PullRequestReview{
				1: {
					mockgithubclient.NewReview(1, "APPROVED", "reviewer1", "Reviewer One", "LGTM"),
					mockgithubclient.NewReview(2, "APPROVED", "reviewer2", "Reviewer Two", "Looks good"),
				},
				2: {
					mockgithubclient.NewReview(3, "COMMENTED", "reviewer3", "Reviewer Three", "Just a few questions..."),
				},
			},
			expectedPRItemTexts: []string{
				"First PR 5 hours ago by Alice (✅ Reviewer One, Reviewer Two)",
				"Second PR 5 hours ago by Bob (💬 Reviewer Three)",
			},
		},
		{
			name:   "update mode fails when fetching individual PR fails",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputRunMode: config.RunModeUpdate,
			},
			mockState: testhelpers.AsPointer(getTestState(GetTestStateOptions{PRNumbers: []int{1, 2}})),
			prByNumber: map[int]*github.PullRequest{
				1: getTestPR(GetTestPROptions{Number: 1, Title: "First PR", AuthorLogin: "alice"}),
			},
			fetchPRErrorByPRNumber: map[int]error{
				2: errors.New("failed to fetch PR"),
			},
			expectedErrorMsg: "failed to fetch PR",
		},
		{
			name:   "update mode fails when artifact listing fails",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputRunMode: config.RunModeUpdate,
			},
			mockState:          testhelpers.AsPointer(getTestState(GetTestStateOptions{PRNumbers: []int{1}})),
			listArtifactsError: errors.New("artifact listing error"),
			expectedErrorMsg:   "artifact listing error",
		},
		{
			name:   "update mode fails when artifact download fails",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputRunMode: config.RunModeUpdate,
			},
			mockState:             testhelpers.AsPointer(getTestState(GetTestStateOptions{PRNumbers: []int{1}})),
			downloadArtifactError: errors.New("http client error"),
			expectedErrorMsg:      "http client error",
		},
		{
			name:   "update mode fails when state artifact is not found",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputRunMode: config.RunModeUpdate,
			},
			mockState:        nil,
			expectedErrorMsg: "no artifacts found with name",
		},
		{
			name:   "update mode fails when updating Slack message fails",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputRunMode: config.RunModeUpdate,
			},
			mockState: testhelpers.AsPointer(getTestState(GetTestStateOptions{PRNumbers: []int{1, 2}})),
			prByNumber: map[int]*github.PullRequest{
				1: getTestPR(GetTestPROptions{Number: 1, Title: "First PR", AuthorLogin: "alice"}),
				2: getTestPR(GetTestPROptions{Number: 2, Title: "Second PR", AuthorLogin: "bob"}),
			},
			updateMessageError: errors.New("slack update failed"),
			expectedErrorMsg:   "failed to update Slack message: slack update failed",
		},
		{
			name:   "update mode with merged and closed PRs shows appropriate status indicators",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputRunMode: config.RunModeUpdate,
			},
			mockState: testhelpers.AsPointer(getTestState(GetTestStateOptions{PRNumbers: []int{1, 2, 3, 4}})),
			prByNumber: map[int]*github.PullRequest{
				1: getTestPR(GetTestPROptions{
					Number:      1,
					Title:       "Open PR with approvals",
					AuthorLogin: "alice",
					State:       "open",
				}),
				2: getTestPR(GetTestPROptions{
					Number:      2,
					Title:       "Merged PR with reviewer",
					AuthorLogin: "bob",
					State:       "closed",
					Merged:      true,
				}),
				3: getTestPR(GetTestPROptions{
					Number:      3,
					Title:       "Closed PR without merge",
					AuthorLogin: "charlie",
					State:       "closed",
					Merged:      false,
				}),
				4: getTestPR(GetTestPROptions{
					Number:      4,
					Title:       "Merged PR without reviewers",
					AuthorLogin: "dave",
					State:       "closed",
					Merged:      true,
				}),
			},
			reviewsByPRNumber: map[int][]*github.PullRequestReview{
				1: {
					mockgithubclient.NewReview(1, "APPROVED", "reviewer1", "Reviewer One", "LGTM"),
				},
				2: {
					mockgithubclient.NewReview(2, "APPROVED", "reviewer2", "Reviewer Two", "Looks good"),
				},
			},
			expectedPRItemTexts: []string{
				"Open PR with approvals 5 hours ago by Alice (✅ Reviewer One)",
				"Merged PR with reviewer 5 hours ago by Bob (✅ Reviewer Two) 🔀",
				"~Closed PR without merge~ 5 hours ago by Charlie",
				"Merged PR without reviewers 5 hours ago by Dave 🔀",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testhelpers.SetTestEnvironment(t, tc.config, tc.configOverrides)

			getGitHubClient := mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
				PRsByNumber:            tc.prByNumber,
				ErrByPRNumber:          tc.fetchPRErrorByPRNumber,
				ReviewsByPRNumber:      tc.reviewsByPRNumber,
				MockStateForUpdateMode: tc.mockState,
				ListArtifactsError:     tc.listArtifactsError,
				DownloadArtifactError:  tc.downloadArtifactError,
			})
			mockSlackAPI := mockslackclient.GetMockSlackAPI(
				nil,
				nil,
				nil,
				tc.updateMessageError,
			)
			getSlackClient := mockslackclient.MakeSlackClientGetter(mockSlackAPI)

			err := main.Run(getGitHubClient, getSlackClient)

			if tc.expectedErrorMsg == "" && err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}
			if tc.expectedErrorMsg != "" && err == nil {
				t.Errorf("Expected error: %v, got no error", tc.expectedErrorMsg)
			}
			if tc.expectedErrorMsg != "" && err != nil && !strings.Contains(err.Error(), tc.expectedErrorMsg) {
				t.Errorf("Expected error message '%v', got: %v", tc.expectedErrorMsg, err)
				t.Logf("Got error: %v", err)
			}
			if tc.expectedErrorMsg != "" {
				return
			}
			if len(tc.expectedPRItemTexts) != mockSlackAPI.UpdatedMessage.Blocks.GetPRCount() {
				t.Errorf(
					"Expected %v PRs to be included in the message (was %v)",
					len(tc.expectedPRItemTexts), mockSlackAPI.UpdatedMessage.Blocks.GetPRCount(),
				)
			}
			if len(tc.expectedPRItemTexts) > 0 {
				missingPRItems := false
				for _, expectedText := range tc.expectedPRItemTexts {
					if !mockSlackAPI.UpdatedMessage.Blocks.SomePRItemTextIsEqualTo(expectedText) {
						t.Errorf(
							"Expected list item text '%s' to be in the updated message blocks", expectedText,
						)
						missingPRItems = true
					}
				}
				if missingPRItems {
					prItems := mockSlackAPI.UpdatedMessage.Blocks.GetAllPRItemTexts()
					t.Logf("Found PR items:")
					for _, prItem := range prItems {
						t.Log(prItem)
					}
				}
			}
		})
	}
}
