package main_test

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
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
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
	"github.com/hellej/pr-slack-reminder-action/testhelpers"
	"github.com/hellej/pr-slack-reminder-action/testhelpers/mockgithubclient"
	"github.com/hellej/pr-slack-reminder-action/testhelpers/mockslackclient"
)

type GetTestPROptions struct {
	Number         int
	Title          string
	HTMLURL        string // left unset by default
	AuthorLogin    string
	AuthorName     string
	Labels         []string
	AgeHours       float32
	Draft          *bool  // nil means unset, github.Ptr(true) means draft, github.Ptr(false) means not draft
	State          string // "open", "closed"
	Merged         bool   // true if PR is merged
	MergedHoursAgo float32
	UpdatedDaysAgo float32 // 0 leaves the update time unset, which reads as unknown activity
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

	var mergedAt *github.Timestamp
	if options.MergedHoursAgo > 0 {
		mergedAt = &github.Timestamp{
			Time: now.Add(-time.Duration(options.MergedHoursAgo * float32(time.Hour))),
		}
	}

	var updatedAt *github.Timestamp
	if options.UpdatedDaysAgo > 0 {
		updatedAt = &github.Timestamp{
			Time: now.Add(-time.Duration(options.UpdatedDaysAgo * float32(24*time.Hour))),
		}
	}

	return &github.PullRequest{
		Number:  &number,
		Title:   &title,
		HTMLURL: &options.HTMLURL,
		User: &github.User{
			Login: &authorLogin,
			Name:  &authorName,
		},
		Labels:    githubLabels,
		CreatedAt: &github.Timestamp{Time: prTime},
		UpdatedAt: updatedAt,
		Draft:     options.Draft,
		State:     &state,
		Merged:    &options.Merged,
		MergedAt:  mergedAt,
	}
}

// GitHub returns a null name for accounts that have not set one.
func withoutAuthorName(pr *github.PullRequest) *github.PullRequest {
	authorWithoutName := *pr.User
	authorWithoutName.Name = nil

	prWithNamelessAuthor := *pr
	prWithNamelessAuthor.User = &authorWithoutName
	return &prWithNamelessAuthor
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
	// Defaults to an hour ago.
	PostedHoursAgo float32
}

func getTestState(options GetTestStateOptions) state.State {
	prRefs := utilities.Map(options.PRNumbers, stateRef)
	postedHoursAgo := options.PostedHoursAgo
	if postedHoursAgo == 0 {
		postedHoursAgo = 1
	}

	return state.State{
		SchemaVersion: 1,
		CreatedAt:     time.Now().Add(-time.Duration(postedHoursAgo * float32(time.Hour))),
		SlackMessage: state.SlackRef{
			ChannelID: "C12345678",
			MessageTS: "1623850245.000200",
		},
		PullRequests: prRefs,
	}
}

func TestScenarios(t *testing.T) {
	testCases := []struct {
		name                       string
		config                     testhelpers.TestConfig
		configOverrides            *map[string]any
		fetchPRsStatus             int
		prServiceError             error
		issueServiceError          error
		prs                        []*github.PullRequest
		prsByRepo                  map[string][]*github.PullRequest
		reviewsByPRNumber          map[int][]*github.PullRequestReview
		timelineCommentsByPRNumber map[int][]*github.IssueComment
		foundSlackChannels         []*mockslackclient.SlackChannel
		findChannelError           error
		sendMessageError           error
		expectedErrorMsg           string
		expectedPRNumbers          []int
		expectedPRItemTexts        []string
		expectedSummary            string
		expectedHeadings           []string // For group-by-repository mode to check repository headings
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
			expectedSummary: "Nothing waiting for review 🎉",
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
			configOverrides:  &map[string]any{config.InputGlobalFilters: "{\"authors\": [\"alice\"], \"ignored-authors\": [\"bob\"]}"},
			expectedErrorMsg: "configuration error: error reading input filters: invalid filters: {\"authors\": [\"alice\"], \"ignored-authors\": [\"bob\"]}, error: cannot use both authors and ignored-authors filters at the same time",
		},
		{
			name:             "invalid global filters input: conflicting labels and ignored-labels",
			config:           testhelpers.GetDefaultConfigMinimal(),
			configOverrides:  &map[string]any{config.InputGlobalFilters: "{\"labels\": [\"infra\"], \"ignored-labels\": [\"infra\"]}"},
			expectedErrorMsg: "configuration error: error reading input filters: invalid filters: {\"labels\": [\"infra\"], \"ignored-labels\": [\"infra\"]}, error: labels filter cannot contain labels that are in ignored-labels filter",
		},
		{
			name:             "invalid repository filters input: invalid mapping",
			config:           testhelpers.GetDefaultConfigMinimal(),
			configOverrides:  &map[string]any{config.InputRepositoryFilters: "asdf"},
			expectedErrorMsg: "configuration error: error reading input repository-filters: invalid mapping format for repository-filters: 'asdf'",
		},
		{
			name:             "invalid repository filters input: conflicting labels and ignored-labels",
			config:           testhelpers.GetDefaultConfigMinimal(),
			configOverrides:  &map[string]any{config.InputRepositoryFilters: "\"test-repo\": {\"labels\": [\"infra\"], \"ignored-labels\": [\"infra\"]}"},
			expectedErrorMsg: "configuration error: error parsing filters for repository \"test-repo\": invalid filters: {\"labels\": [\"infra\"], \"ignored-labels\": [\"infra\"]}, error: labels filter cannot contain labels that are in ignored-labels filter",
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
			expectedErrorMsg: "error fetching pull requests: GraphQL request failed with status 500",
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
			configOverrides: &map[string]any{config.InputGlobalFilters: "{\"ignored-labels\": [\"label-to-ignore\"]}"},
			prs:             getTestPRs(GetTestPRsOptions{Labels: []string{"label-to-ignore"}}).PRs,
			expectedSummary: "", // no message should be sent
		},
		{
			name:            "PRs by one user filtered out",
			config:          testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{config.InputGlobalFilters: "{\"ignored-authors\": [\"alice\"]}"},
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
			configOverrides: &map[string]any{config.InputGlobalFilters: "{\"ignored-authors\": [\"lilo\"]}"},
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
				config.InputRepositoryFilters:  "repo1: {\"ignored-authors\": [\"alice\"]}",
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
				config.InputRepositoryFilters:  "some-org/repo1: {\"ignored-authors\": [\"alice\"]}",
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
				config.InputGlobalFilters:      "{\"ignored-authors\": [\"alice\"]}",
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
			},
			prs:               getTestPRs(GetTestPRsOptions{Labels: []string{"feature"}}).PRs,
			expectedPRNumbers: getTestPRs(GetTestPRsOptions{}).PRNumbers,
			expectedPRItemTexts: []string{
				"This is a test PR 5 minutes ago by Stitch",
				"This PR was created 3 hours ago and contains important changes 3 hours ago by U2234567890",
				"This PR has the same time as PR2 but a longer title 3 hours ago by U2234567890",
				"This PR is getting old and needs attention 🚨 1 day old by U3234567890",
				"This is a big PR that no one dares to review 🚨 2 days old by Jim",
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
					mockgithubclient.NewReview("reviewer1", "", "APPROVED"),
					mockgithubclient.NewReview("reviewer2", "", "APPROVED"),
				},
				*getTestPRs(GetTestPRsOptions{}).PR2.Number: {
					mockgithubclient.NewReview("reviewer1", "", "COMMENTED"),
					mockgithubclient.NewReview("reviewer2", "", "COMMENTED"),
				},
				*getTestPRs(GetTestPRsOptions{}).PR3.Number: {
					mockgithubclient.NewReview("reviewer3", "", "COMMENTED"),
				},
				*getTestPRs(GetTestPRsOptions{}).PR4.Number: {
					mockgithubclient.NewReview("reviewer3", "", "COMMENTED"),
					mockgithubclient.NewReview("reviewer3", "", "COMMENTED"), // duplicate review by reviewer3 should be omitted
					mockgithubclient.NewReview("reviewer2", "", "APPROVED"),
					mockgithubclient.NewReview("reviewer2", "", "APPROVED"), // duplicate approval by reviewer2 should be omitted
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
			expectedHeadings:  []string{"test-repo:"},
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
			expectedHeadings:  []string{"repo1:", "repo2:"},
		},
		{
			name:   "reviews by bots and author are excluded from review status",
			config: testhelpers.GetDefaultConfigMinimal(),
			prs: []*github.PullRequest{
				getTestPR(GetTestPROptions{Number: 1, Title: "PR with bot and human reviewers", AuthorLogin: "alice"}),
			},
			reviewsByPRNumber: map[int][]*github.PullRequestReview{
				1: {
					mockgithubclient.NewReview("human-reviewer", "Human Reviewer", "COMMENTED", "User"),
					mockgithubclient.NewReview("alice", "Alice", "COMMENTED", "User"),
					mockgithubclient.NewReview("dependabot", "Dependabot", "COMMENTED", "Bot"),
					mockgithubclient.NewReview("codecov", "Codecov", "APPROVED", "Bot"),
				},
			},
			expectedPRNumbers: []int{1},
			expectedSummary:   "1 open PR is waiting for attention 👀",
			expectedPRItemTexts: []string{
				"PR with bot and human reviewers 5 hours ago by Alice (💬 Human Reviewer)",
			},
		},
		{
			name:   "snoozed PR is excluded from message",
			config: testhelpers.GetDefaultConfigMinimal(),
			prs: []*github.PullRequest{
				getTestPR(GetTestPROptions{Number: 1, Title: "Active PR", AuthorLogin: "alice"}),
				getTestPR(GetTestPROptions{Number: 2, Title: "Snoozed PR", AuthorLogin: "bob"}),
				getTestPR(GetTestPROptions{Number: 3, Title: "Another active PR", AuthorLogin: "charlie"}),
			},
			timelineCommentsByPRNumber: map[int][]*github.IssueComment{
				2: {
					mockgithubclient.NewTimelineComment("bob", "", "/snooze PR reminder for 7 days", now.Add(-1*time.Hour)),
				},
			},
			expectedPRNumbers: []int{1, 3},
			expectedSummary:   "2 open PRs are waiting for attention 👀",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testhelpers.SetTestEnvironment(t, tc.config, tc.configOverrides)

			getGitHubClient := mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
				PRs:                        tc.prs,
				PRsByRepo:                  tc.prsByRepo,
				ListPRsResponseStatus:      cmp.Or(tc.fetchPRsStatus, 200),
				ReviewsByPRNumber:          tc.reviewsByPRNumber,
				TimelineCommentsByPRNumber: tc.timelineCommentsByPRNumber,
				PRServiceError:             tc.prServiceError,
				IssueServiceError:          tc.issueServiceError,
			})
			mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{
				SlackChannels:    tc.foundSlackChannels,
				FindChannelError: tc.findChannelError,
				PostMessageError: tc.sendMessageError,
			})
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
	mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{})

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

// State that points at a message Slack never accepted would make the next update run edit
// somebody else's message, or nothing at all.
func TestPostModeSavesNoStateWhenSendFails(t *testing.T) {
	stateFilePath := filepath.Join(t.TempDir(), stateFileName)
	testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &map[string]any{
		config.InputRunMode:     config.RunModePost,
		config.EnvStateFilePath: stateFilePath,
	})

	err := main.Run(
		mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
			PRs: getTestPRs(GetTestPRsOptions{}).PRs,
		}),
		mockslackclient.MakeSlackClientGetter(
			mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{
				PostMessageError: errors.New("error in sending Slack message"),
			}),
		),
	)

	if err == nil {
		t.Fatal("Expected Run to fail when sending the message fails")
	}
	if _, statErr := os.Stat(stateFilePath); !os.IsNotExist(statErr) {
		t.Errorf("Expected no state file at %s, got: %v", stateFilePath, statErr)
	}
}

// Update mode tracks the PR set of the message it edits, so the state it writes back has to be
// the loaded one, not the PRs that survived this run's filters.
func TestUpdateModeSavesTheLoadedState(t *testing.T) {
	stateFilePath := filepath.Join(t.TempDir(), stateFileName)
	testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &map[string]any{
		config.InputRunMode:       config.RunModeUpdate,
		config.EnvStateFilePath:   stateFilePath,
		config.InputGlobalFilters: "{\"ignored-authors\": [\"bob\"]}",
	})
	loadedState := getTestState(GetTestStateOptions{PRNumbers: []int{1, 2}})

	err := main.Run(
		mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
			PRsByNumber: map[int]*github.PullRequest{
				1: getTestPR(GetTestPROptions{Number: 1, Title: "Surviving PR", AuthorLogin: "alice"}),
				2: getTestPR(GetTestPROptions{Number: 2, Title: "Filtered out PR", AuthorLogin: "bob"}),
			},
			MockStateForUpdateMode: &loadedState,
		}),
		mockslackclient.MakeSlackClientGetter(
			mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{}),
		),
	)

	if err != nil {
		t.Fatalf("Expected Run to succeed, got error: %v", err)
	}
	var savedState state.State
	if loadErr := testhelpers.LoadJSONFromFile(stateFilePath, &savedState); loadErr != nil {
		t.Fatalf("Failed to load the saved state file: %v", loadErr)
	}

	savedPRNumbers := utilities.Map(savedState.PullRequests, func(ref models.PullRequestRef) int {
		return ref.Number
	})
	if !slices.Equal(savedPRNumbers, []int{1, 2}) {
		t.Errorf("Expected the loaded PRs 1 and 2 in the saved state, got %v", savedPRNumbers)
	}
	if savedState.SlackMessage != loadedState.SlackMessage {
		t.Errorf(
			"Expected the loaded Slack message ref %+v, got %+v",
			loadedState.SlackMessage, savedState.SlackMessage,
		)
	}
	if !savedState.CreatedAt.Equal(loadedState.CreatedAt) {
		t.Errorf(
			"Expected the loaded CreatedAt %v, got %v", loadedState.CreatedAt, savedState.CreatedAt,
		)
	}
	if savedState.SchemaVersion != 1 {
		t.Errorf("Expected schema version 1 in the saved state, got %d", savedState.SchemaVersion)
	}
}

func TestUpdateModeStateSavingOnEarlyReturns(t *testing.T) {
	testCases := []struct {
		name              string
		configOverrides   map[string]any
		prNumbersInState  []int
		prByNumber        map[int]*github.PullRequest
		listArtifactError error
		expectStateSaved  bool
	}{
		{
			name:             "message deleted because all PRs are gone",
			configOverrides:  map[string]any{config.InputGlobalFilters: "{\"ignored-authors\": [\"alice\"]}"},
			prNumbersInState: []int{1},
			prByNumber: map[int]*github.PullRequest{
				1: getTestPR(GetTestPROptions{Number: 1, Title: "Filtered out PR", AuthorLogin: "alice"}),
			},
			expectStateSaved: true,
		},
		{
			name:             "loaded state has no PRs",
			prNumbersInState: []int{},
			expectStateSaved: true,
		},
		{
			name:              "state load fails",
			prNumbersInState:  []int{1},
			listArtifactError: errors.New("artifact listing error"),
			expectStateSaved:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stateFilePath := filepath.Join(t.TempDir(), stateFileName)
			overrides := map[string]any{
				config.InputRunMode:     config.RunModeUpdate,
				config.EnvStateFilePath: stateFilePath,
			}
			maps.Copy(overrides, tc.configOverrides)
			testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &overrides)
			loadedState := getTestState(GetTestStateOptions{PRNumbers: tc.prNumbersInState})

			err := main.Run(
				mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
					PRsByNumber:            tc.prByNumber,
					MockStateForUpdateMode: &loadedState,
					ListArtifactsError:     tc.listArtifactError,
				}),
				mockslackclient.MakeSlackClientGetter(
					mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{}),
				),
			)

			if tc.listArtifactError == nil && err != nil {
				t.Fatalf("Expected Run to succeed, got error: %v", err)
			}
			_, statErr := os.Stat(stateFilePath)
			if tc.expectStateSaved && statErr != nil {
				t.Errorf("Expected the state file to be saved: %v", statErr)
			}
			if !tc.expectStateSaved && !os.IsNotExist(statErr) {
				t.Errorf("Expected no state file at %s, got: %v", stateFilePath, statErr)
			}
		})
	}
}

// The PRs the open-PR fetch returns in update mode: the named state PRs, rendered from the same
// fixtures the tracked-PR fetch renders, plus the PRs opened since the message was posted.
func openPRsOfFetch(
	prByNumber map[int]*github.PullRequest,
	openPRNumbers []int,
	openPRsNotInState []*github.PullRequest,
) []*github.PullRequest {
	openPRs := make([]*github.PullRequest, 0, len(openPRNumbers)+len(openPRsNotInState))
	for _, number := range openPRNumbers {
		openPRs = append(openPRs, prByNumber[number])
	}
	return append(openPRs, openPRsNotInState...)
}

func TestScenariosUpdateMode(t *testing.T) {
	testCases := []struct {
		name            string
		config          testhelpers.TestConfig
		configOverrides *map[string]any
		mockState       *state.State
		prByNumber      map[int]*github.PullRequest
		// The open-PR fetch is live, so it returns the state PRs that are still open, by number,
		// and any PR opened since the message was posted.
		openPRNumbers         []int
		openPRsNotInState     []*github.PullRequest
		mergedPRsFromSearch   []*github.PullRequest
		mergedPRsSearchError  error
		reviewsByPRNumber     map[int][]*github.PullRequestReview
		listArtifactsError    error
		downloadArtifactError error
		updateMessageError    error
		deleteMessageError    error
		expectedErrorMsg      string
		expectedPRItemTexts   []string
		expectMessageDeleted  bool
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
			// A message posted with no PRs still has the live ones to show.
			name:   "update mode with an empty state lists the PRs open now",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputRunMode: config.RunModeUpdate,
			},
			mockState: testhelpers.AsPointer(getTestState(GetTestStateOptions{PRNumbers: []int{}})),
			openPRsNotInState: []*github.PullRequest{
				getTestPR(GetTestPROptions{Number: 1, Title: "Opened since the post", AuthorLogin: "alice"}),
			},
			expectedPRItemTexts: []string{"Opened since the post 5 hours ago by Alice"},
		},
		{
			name:   "update mode with all PRs filtered out deletes message",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputRunMode:       config.RunModeUpdate,
				config.InputGlobalFilters: "{\"ignored-authors\": [\"alice\", \"bob\"]}",
			},
			mockState: testhelpers.AsPointer(getTestState(GetTestStateOptions{PRNumbers: []int{1, 2}})),
			prByNumber: map[int]*github.PullRequest{
				1: getTestPR(GetTestPROptions{Number: 1, Title: "First PR", AuthorLogin: "alice"}),
				2: getTestPR(GetTestPROptions{Number: 2, Title: "Second PR", AuthorLogin: "bob"}),
			},
			openPRNumbers:        []int{1, 2},
			expectMessageDeleted: true,
		},
		{
			name:   "update mode with all PRs filtered out and no-prs-message updates message",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputRunMode:       config.RunModeUpdate,
				config.InputGlobalFilters: "{\"ignored-authors\": [\"alice\", \"bob\"]}",
				config.InputNoPRsMessage:  "All PRs have been resolved! 🎉",
			},
			mockState: testhelpers.AsPointer(getTestState(GetTestStateOptions{PRNumbers: []int{1, 2}})),
			prByNumber: map[int]*github.PullRequest{
				1: getTestPR(GetTestPROptions{Number: 1, Title: "First PR", AuthorLogin: "alice"}),
				2: getTestPR(GetTestPROptions{Number: 2, Title: "Second PR", AuthorLogin: "bob"}),
			},
		},
		{
			name:   "update mode with all PRs as drafts deletes message",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputRunMode: config.RunModeUpdate,
			},
			mockState: testhelpers.AsPointer(getTestState(GetTestStateOptions{PRNumbers: []int{1, 2}})),
			prByNumber: map[int]*github.PullRequest{
				1: getTestPR(GetTestPROptions{Number: 1, Title: "Draft PR 1", AuthorLogin: "alice", Draft: github.Ptr(true)}),
				2: getTestPR(GetTestPROptions{Number: 2, Title: "Draft PR 2", AuthorLogin: "bob", Draft: github.Ptr(true)}),
			},
			openPRNumbers:        []int{1, 2},
			expectMessageDeleted: true,
		},
		{
			name:   "update mode handles delete failure gracefully",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputRunMode:       config.RunModeUpdate,
				config.InputGlobalFilters: "{\"ignored-authors\": [\"alice\"]}",
			},
			mockState: testhelpers.AsPointer(getTestState(GetTestStateOptions{PRNumbers: []int{1}})),
			prByNumber: map[int]*github.PullRequest{
				1: getTestPR(GetTestPROptions{Number: 1, Title: "First PR", AuthorLogin: "alice"}),
			},
			deleteMessageError:   errors.New("slack delete failed"),
			expectMessageDeleted: true,
		},
		{
			name:   "update mode with message_not_found error exits gracefully",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputRunMode:       config.RunModeUpdate,
				config.InputGlobalFilters: "{\"ignored-authors\": [\"alice\"]}",
			},
			mockState: testhelpers.AsPointer(getTestState(GetTestStateOptions{PRNumbers: []int{1}})),
			prByNumber: map[int]*github.PullRequest{
				1: getTestPR(GetTestPROptions{Number: 1, Title: "First PR", AuthorLogin: "alice"}),
			},
			deleteMessageError:   errors.New("message_not_found"),
			expectMessageDeleted: true,
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
			openPRNumbers: []int{1, 2},
			reviewsByPRNumber: map[int][]*github.PullRequestReview{
				1: {
					mockgithubclient.NewReview("reviewer1", "Reviewer One", "APPROVED"),
					mockgithubclient.NewReview("reviewer2", "Reviewer Two", "APPROVED"),
				},
				2: {
					mockgithubclient.NewReview("reviewer3", "Reviewer Three", "COMMENTED"),
				},
			},
			expectedPRItemTexts: []string{
				"First PR 5 hours ago by Alice (✅ Reviewer One, Reviewer Two)",
				"Second PR 5 hours ago by Bob (💬 Reviewer Three)",
			},
		},
		{
			name:   "update mode renders the login of an author without a display name",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputRunMode: config.RunModeUpdate,
			},
			mockState: testhelpers.AsPointer(getTestState(GetTestStateOptions{PRNumbers: []int{1}})),
			prByNumber: map[int]*github.PullRequest{
				1: withoutAuthorName(
					getTestPR(GetTestPROptions{Number: 1, Title: "First PR", AuthorLogin: "nameless-author"}),
				),
			},
			openPRNumbers:       []int{1},
			expectedPRItemTexts: []string{"First PR 5 hours ago by nameless-author"},
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
			openPRNumbers:      []int{1, 2},
			updateMessageError: errors.New("slack update failed"),
			expectedErrorMsg:   "failed to update Slack message: slack update failed",
		},
		{
			// The message the reminder exists to show is the one it would otherwise delete here:
			// every PR it was posted with has landed, and no no-prs-message is configured.
			name:   "update mode keeps a message whose only rows are merged PRs",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputRunMode: config.RunModeUpdate,
			},
			mockState: testhelpers.AsPointer(getTestState(GetTestStateOptions{PRNumbers: []int{1, 2}})),
			prByNumber: map[int]*github.PullRequest{
				1: getTestPR(GetTestPROptions{
					Number: 1, Title: "First PR", AuthorLogin: "alice",
					State: "closed", Merged: true, MergedHoursAgo: 3,
				}),
				2: getTestPR(GetTestPROptions{
					Number: 2, Title: "Second PR", AuthorLogin: "bob",
					State: "closed", Merged: true, MergedHoursAgo: 1,
				}),
			},
			expectedPRItemTexts: []string{
				"Second PR merged 1 hour ago by Bob",
				"First PR merged 3 hours ago by Alice",
			},
		},
		{
			name:   "update mode lists merged PRs in their own section and drops closed ones",
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
					Number:         2,
					Title:          "Merged PR with reviewer",
					AuthorLogin:    "bob",
					State:          "closed",
					Merged:         true,
					MergedHoursAgo: 6,
				}),
				3: getTestPR(GetTestPROptions{
					Number:      3,
					Title:       "Closed PR without merge",
					AuthorLogin: "charlie",
					State:       "closed",
					Merged:      false,
				}),
				4: getTestPR(GetTestPROptions{
					Number:         4,
					Title:          "Merged PR without reviewers",
					AuthorLogin:    "dave",
					State:          "closed",
					Merged:         true,
					MergedHoursAgo: 2,
				}),
			},
			openPRNumbers: []int{1},
			reviewsByPRNumber: map[int][]*github.PullRequestReview{
				1: {
					mockgithubclient.NewReview("reviewer1", "Reviewer One", "APPROVED"),
				},
				2: {
					mockgithubclient.NewReview("reviewer2", "Reviewer Two", "APPROVED"),
				},
			},
			// The closed-but-not-merged PR reaches no section.
			expectedPRItemTexts: []string{
				"Open PR with approvals 5 hours ago by Alice (✅ Reviewer One)",
				"Merged PR without reviewers merged 2 hours ago by Dave",
				"Merged PR with reviewer merged 6 hours ago by Bob (✅ Reviewer Two)",
			},
		},
		{
			name:   "update mode lists the PRs open right now, not the ones in state",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputRunMode: config.RunModeUpdate,
			},
			mockState: testhelpers.AsPointer(getTestState(GetTestStateOptions{PRNumbers: []int{1, 2}})),
			prByNumber: map[int]*github.PullRequest{
				1: getTestPR(GetTestPROptions{Number: 1, Title: "Still open PR", AuthorLogin: "alice"}),
				2: getTestPR(GetTestPROptions{
					Number: 2, Title: "Closed since the post", AuthorLogin: "bob", State: "closed",
				}),
			},
			openPRNumbers: []int{1},
			openPRsNotInState: []*github.PullRequest{
				getTestPR(GetTestPROptions{Number: 3, Title: "Opened since the post", AuthorLogin: "carol"}),
			},
			expectedPRItemTexts: []string{
				"Still open PR 5 hours ago by Alice",
				"Opened since the post 5 hours ago by Carol",
			},
		},
		{
			// 7 days is githubclient.RecentlyMergedWindow, so the merged fetch drops this PR even
			// though the search returns it. Only the state artifact can still name it.
			name:   "update mode keeps a state PR merged before the recently merged window",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputRunMode: config.RunModeUpdate,
			},
			mockState: testhelpers.AsPointer(getTestState(GetTestStateOptions{PRNumbers: []int{1}})),
			prByNumber: map[int]*github.PullRequest{
				1: getTestPR(GetTestPROptions{
					Number: 1, Title: "Merged last week", AuthorLogin: "alice",
					State: "closed", Merged: true, MergedHoursAgo: 7*24 + 1,
				}),
			},
			mergedPRsFromSearch: []*github.PullRequest{
				getTestPR(GetTestPROptions{
					Number: 1, Title: "Merged last week", AuthorLogin: "alice",
					State: "closed", Merged: true, MergedHoursAgo: 7*24 + 1,
				}),
			},
			expectedPRItemTexts: []string{"Merged last week merged 7 days ago by Alice"},
		},
		{
			// A post made with no open PRs and a no-prs-message saves a state with no PRs, so an
			// update run can find one. With the input gone by then, nothing is left to show.
			name:   "update mode with an empty state and nothing open deletes the message",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputRunMode: config.RunModeUpdate,
			},
			mockState:            testhelpers.AsPointer(getTestState(GetTestStateOptions{PRNumbers: []int{}})),
			expectMessageDeleted: true,
		},
		{
			// A failed merged search leaves the run unable to tell an empty day from an outage,
			// and only one of the two should remove a message this run cannot rebuild.
			name:   "update mode keeps the message when the merged PR search failed",
			config: testhelpers.GetDefaultConfigMinimal(),
			configOverrides: &map[string]any{
				config.InputRunMode: config.RunModeUpdate,
			},
			mockState: testhelpers.AsPointer(getTestState(GetTestStateOptions{PRNumbers: []int{1}})),
			prByNumber: map[int]*github.PullRequest{
				1: getTestPR(GetTestPROptions{
					Number: 1, Title: "Closed without merging", AuthorLogin: "alice", State: "closed",
				}),
			},
			mergedPRsSearchError: errors.New("merged PR search failed"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testhelpers.SetTestEnvironment(t, tc.config, tc.configOverrides)

			getGitHubClient := mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
				PRsByNumber:            tc.prByNumber,
				PRs:                    openPRsOfFetch(tc.prByNumber, tc.openPRNumbers, tc.openPRsNotInState),
				MergedPRs:              tc.mergedPRsFromSearch,
				MergedPRsSearchError:   tc.mergedPRsSearchError,
				ReviewsByPRNumber:      tc.reviewsByPRNumber,
				MockStateForUpdateMode: tc.mockState,
				ListArtifactsError:     tc.listArtifactsError,
				DownloadArtifactError:  tc.downloadArtifactError,
			})
			mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{
				UpdateMessageError: tc.updateMessageError,
				DeleteMessageError: tc.deleteMessageError,
			})
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

			if tc.expectMessageDeleted {
				if mockSlackAPI.DeletedMessage.ChannelID == "" {
					t.Error("Expected message to be deleted, but DeleteMessage was not called")
				}
				expectedChannelID := "C12345678"
				expectedTimestamp := "1623850245.000200"
				if mockSlackAPI.DeletedMessage.ChannelID != expectedChannelID {
					t.Errorf("Expected deleted message channel ID to be %s, got %s",
						expectedChannelID, mockSlackAPI.DeletedMessage.ChannelID)
				}
				if mockSlackAPI.DeletedMessage.Timestamp != expectedTimestamp {
					t.Errorf("Expected deleted message timestamp to be %s, got %s",
						expectedTimestamp, mockSlackAPI.DeletedMessage.Timestamp)
				}
				if mockSlackAPI.UpdatedMessage.Blocks.GetPRCount() != 0 {
					t.Errorf("Expected no PRs in updated message when deleting, got %d",
						mockSlackAPI.UpdatedMessage.Blocks.GetPRCount())
				}
				return
			}

			if mockSlackAPI.DeletedMessage.ChannelID != "" {
				t.Error("Expected the message to be kept, but DeleteMessage was called")
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

// Both fetches are made before the run-mode switch, so an update run makes them with the canvas
// off too, and its message is built from them.
func TestUpdateModeFetchesOpenAndMergedPRsWithTheCanvasDisabled(t *testing.T) {
	testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &map[string]any{
		config.InputRunMode: config.RunModeUpdate,
	})
	trackedPR := getTestPR(GetTestPROptions{Number: 1, Title: "Tracked PR", AuthorLogin: "alice"})
	openPRNotInState := getTestPR(GetTestPROptions{Number: 2, Title: "Open PR not in state", AuthorLogin: "bob"})
	loadedState := getTestState(GetTestStateOptions{PRNumbers: []int{1}})
	recording := mockgithubclient.FetchRecording{}

	mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{})
	err := main.Run(
		mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
			PRsByNumber: map[int]*github.PullRequest{1: trackedPR},
			PRs:         []*github.PullRequest{trackedPR, openPRNotInState},
			MergedPRs: []*github.PullRequest{getTestPR(GetTestPROptions{
				Number: 3, Title: "Merged PR", AuthorLogin: "carol", MergedHoursAgo: 2,
			})},
			MockStateForUpdateMode: &loadedState,
			Recording:              &recording,
		}),
		mockslackclient.MakeSlackClientGetter(mockSlackAPI),
	)

	if err != nil {
		t.Fatalf("Expected Run to succeed, got error: %v", err)
	}
	if recording.OpenPRFetches != 1 {
		t.Errorf("Expected 1 open PR fetch, got %d", recording.OpenPRFetches)
	}
	if recording.MergedPRFetches != 1 {
		t.Errorf("Expected 1 merged PR fetch, got %d", recording.MergedPRFetches)
	}
	updatedMessage := mockSlackAPI.UpdatedMessage.Blocks
	if !updatedMessage.SomePRItemContainsText("Tracked PR") {
		t.Error("Expected the state-tracked PR in the updated message")
	}
	if !updatedMessage.SomePRItemContainsText("Open PR not in state") {
		t.Error("Expected a PR opened since the post in the updated message")
	}
	if !updatedMessage.SomePRItemContainsText("Merged PR") {
		t.Error("Expected the searched merged PR in the updated message")
	}
}

func stateRef(number int) models.PullRequestRef {
	return models.PullRequestRef{
		Repository: models.Repository{Owner: "test-org", Name: "test-repo"},
		Number:     number,
	}
}

func setUpdateModeEnvironment(t *testing.T) {
	testhelpers.SetTestEnvironment(t, testhelpers.GetDefaultConfigMinimal(), &map[string]any{
		config.InputRunMode: config.RunModeUpdate,
	})
}

func TestUpdateModeSkipsTheTrackedPRFetchWhenTheStatePRsAreStillOpen(t *testing.T) {
	setUpdateModeEnvironment(t)
	openPR1 := getTestPR(GetTestPROptions{Number: 1, Title: "Still open PR", AuthorLogin: "alice"})
	openPR2 := getTestPR(GetTestPROptions{Number: 2, Title: "Also still open PR", AuthorLogin: "bob"})
	loadedState := getTestState(GetTestStateOptions{PRNumbers: []int{1, 2}})
	recording := mockgithubclient.FetchRecording{}

	mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{})
	err := main.Run(
		mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
			PRs:                    []*github.PullRequest{openPR1, openPR2},
			MockStateForUpdateMode: &loadedState,
			Recording:              &recording,
		}),
		mockslackclient.MakeSlackClientGetter(mockSlackAPI),
	)

	if err != nil {
		t.Fatalf("Expected Run to succeed, got error: %v", err)
	}
	if len(recording.GetPRsRequests) != 0 {
		t.Errorf("Expected no GetPRs request, got %v", recording.GetPRsRequests)
	}
	if !mockSlackAPI.UpdatedMessage.Blocks.SomePRItemContainsText("Still open PR") {
		t.Error("Expected the state PR in the updated message, rendered from the open PR fetch")
	}
}

func TestUpdateModeSkipsTheTrackedPRFetchWhenAStatePRMergedInsideTheWindow(t *testing.T) {
	setUpdateModeEnvironment(t)
	mergedStatePR := getTestPR(GetTestPROptions{
		Number: 1, Title: "Merged state PR", AuthorLogin: "alice",
		State: "closed", Merged: true, MergedHoursAgo: 2,
	})
	loadedState := getTestState(GetTestStateOptions{PRNumbers: []int{1}})
	recording := mockgithubclient.FetchRecording{}

	mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{})
	err := main.Run(
		mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
			PRs:                    []*github.PullRequest{getTestPR(GetTestPROptions{Number: 9, Title: "Open PR"})},
			MergedPRs:              []*github.PullRequest{mergedStatePR},
			MockStateForUpdateMode: &loadedState,
			Recording:              &recording,
		}),
		mockslackclient.MakeSlackClientGetter(mockSlackAPI),
	)

	if err != nil {
		t.Fatalf("Expected Run to succeed, got error: %v", err)
	}
	if len(recording.GetPRsRequests) != 0 {
		t.Errorf("Expected no GetPRs request, got %v", recording.GetPRsRequests)
	}
	if !mockSlackAPI.UpdatedMessage.Blocks.SomePRItemContainsText("Merged state PR") {
		t.Error("Expected the merged state PR in the updated message, taken from the merged PR fetch")
	}
}

// A state PR merged before the merged fetch's window reaches neither fetch, so the run asks for
// that one ref alone, and the answer carries its reviewers.
func TestUpdateModeFetchesOnlyTheStatePRsNeitherFetchResolved(t *testing.T) {
	setUpdateModeEnvironment(t)
	openStatePR := getTestPR(GetTestPROptions{Number: 1, Title: "Still open PR", AuthorLogin: "alice"})
	longAgoMergedStatePR := getTestPR(GetTestPROptions{
		Number: 2, Title: "Merged nine days ago", AuthorLogin: "bob",
		State: "closed", Merged: true, MergedHoursAgo: 9 * 24,
	})
	loadedState := getTestState(GetTestStateOptions{PRNumbers: []int{1, 2}})
	recording := mockgithubclient.FetchRecording{}

	mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{})
	err := main.Run(
		mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
			PRs:         []*github.PullRequest{openStatePR},
			MergedPRs:   []*github.PullRequest{longAgoMergedStatePR},
			PRsByNumber: map[int]*github.PullRequest{2: longAgoMergedStatePR},
			ReviewsByPRNumber: map[int][]*github.PullRequestReview{
				2: {mockgithubclient.NewReview("dana", "Dana", "APPROVED")},
			},
			MockStateForUpdateMode: &loadedState,
			Recording:              &recording,
		}),
		mockslackclient.MakeSlackClientGetter(mockSlackAPI),
	)

	if err != nil {
		t.Fatalf("Expected Run to succeed, got error: %v", err)
	}
	expectedRequests := [][]models.PullRequestRef{{stateRef(2)}}
	if !slices.EqualFunc(recording.GetPRsRequests, expectedRequests, slices.Equal) {
		t.Errorf("Expected GetPRs requests %v, got %v", expectedRequests, recording.GetPRsRequests)
	}
	if !mockSlackAPI.UpdatedMessage.Blocks.SomePRItemContainsText("Merged nine days ago") {
		t.Error("Expected the long ago merged state PR in the updated message")
	}
	if !mockSlackAPI.UpdatedMessage.Blocks.SomePRItemContainsText("✅ Dana") {
		t.Errorf(
			"Expected the fetched merged PR to name its approver, got items %v",
			mockSlackAPI.UpdatedMessage.Blocks.GetAllPRItemTexts(),
		)
	}
}

// The cap of 3 applies to the untracked merges alone, so a state PR the merged fetch resolved
// has to arrive as tracked: counted as untracked it would fall behind the three newest merges.
func TestUpdateModeKeepsAStatePRTheMergedFetchResolvedPastTheUntrackedCap(t *testing.T) {
	setUpdateModeEnvironment(t)
	mergedStatePR := getTestPR(GetTestPROptions{
		Number: 1, Title: "Merged state PR", AuthorLogin: "alice",
		State: "closed", Merged: true, MergedHoursAgo: 30,
	})
	newerMerges := []*github.PullRequest{}
	for hoursAgo := 1; hoursAgo <= 4; hoursAgo++ {
		newerMerges = append(newerMerges, getTestPR(GetTestPROptions{
			Number: 10 + hoursAgo, Title: fmt.Sprintf("Merged %dh ago", hoursAgo),
			AuthorLogin: "bob", State: "closed", Merged: true, MergedHoursAgo: float32(hoursAgo),
		}))
	}
	loadedState := getTestState(GetTestStateOptions{PRNumbers: []int{1}, PostedHoursAgo: 0.5})
	recording := mockgithubclient.FetchRecording{}

	mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{})
	err := main.Run(
		mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
			MergedPRs:              append([]*github.PullRequest{mergedStatePR}, newerMerges...),
			MockStateForUpdateMode: &loadedState,
			Recording:              &recording,
		}),
		mockslackclient.MakeSlackClientGetter(mockSlackAPI),
	)

	if err != nil {
		t.Fatalf("Expected Run to succeed, got error: %v", err)
	}
	if len(recording.GetPRsRequests) != 0 {
		t.Errorf("Expected no GetPRs request, got %v", recording.GetPRsRequests)
	}
	updatedMessage := mockSlackAPI.UpdatedMessage.Blocks
	if !updatedMessage.SomePRItemContainsText("Merged state PR") {
		t.Errorf(
			"Expected the tracked merge to survive the untracked cap, got items %v",
			updatedMessage.GetAllPRItemTexts(),
		)
	}
	if updatedMessage.SomePRItemContainsText("Merged 4h ago") {
		t.Error("Expected the 4th newest untracked merge to be dropped by the cap")
	}
	if updatedMessage.GetPRCount() != 4 {
		t.Errorf("Expected 4 merged PRs in the message, got %d", updatedMessage.GetPRCount())
	}
}

// One more merge since the post than the untracked cap, so a capped run drops the oldest.
func TestUpdateModeShowsEveryPRMergedSinceThePost(t *testing.T) {
	setUpdateModeEnvironment(t)
	mergesSincePost := []*github.PullRequest{}
	for hoursAgo := 1; hoursAgo <= 4; hoursAgo++ {
		mergesSincePost = append(mergesSincePost, getTestPR(GetTestPROptions{
			Number: 10 + hoursAgo, Title: fmt.Sprintf("Merged %dh ago", hoursAgo),
			AuthorLogin: "bob", State: "closed", Merged: true, MergedHoursAgo: float32(hoursAgo),
		}))
	}
	loadedState := getTestState(GetTestStateOptions{PostedHoursAgo: 5})

	mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{})
	err := main.Run(
		mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
			MergedPRs:              mergesSincePost,
			MockStateForUpdateMode: &loadedState,
		}),
		mockslackclient.MakeSlackClientGetter(mockSlackAPI),
	)

	if err != nil {
		t.Fatalf("Expected Run to succeed, got error: %v", err)
	}
	updatedMessage := mockSlackAPI.UpdatedMessage.Blocks
	if !updatedMessage.SomePRItemContainsText("Merged 4h ago") {
		t.Errorf(
			"Expected the oldest merge since the post to be kept, got items %v",
			updatedMessage.GetAllPRItemTexts(),
		)
	}
	if updatedMessage.GetPRCount() != 4 {
		t.Errorf("Expected 4 merged PRs in the message, got %d", updatedMessage.GetPRCount())
	}
}

func TestUpdateModeEditsTheMessageWhenTheTrackedPRFetchFails(t *testing.T) {
	setUpdateModeEnvironment(t)
	loadedState := getTestState(GetTestStateOptions{PRNumbers: []int{2}})

	mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{})
	err := main.Run(
		mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
			PRs: []*github.PullRequest{
				getTestPR(GetTestPROptions{Number: 1, Title: "Open PR not in state", AuthorLogin: "alice"}),
			},
			ErrByPRNumber:          map[int]error{2: errors.New("tracked PR fetch failed")},
			MockStateForUpdateMode: &loadedState,
		}),
		mockslackclient.MakeSlackClientGetter(mockSlackAPI),
	)

	if err != nil {
		t.Fatalf("Expected Run to succeed despite the tracked PR fetch failure, got error: %v", err)
	}
	if mockSlackAPI.DeletedMessage.ChannelID != "" {
		t.Error("Expected the message to be kept, but DeleteMessage was called")
	}
	if !mockSlackAPI.UpdatedMessage.Blocks.SomePRItemContainsText("Open PR not in state") {
		t.Error("Expected the message to be edited with the open PRs the run did fetch")
	}
}

// Without the residue, an empty message is a fetch outage as much as an empty day.
func TestUpdateModeKeepsTheMessageWhenTheTrackedPRFetchFailsAndNothingElseIsLeft(t *testing.T) {
	setUpdateModeEnvironment(t)
	loadedState := getTestState(GetTestStateOptions{PRNumbers: []int{2}})

	mockSlackAPI := mockslackclient.GetMockSlackAPI(mockslackclient.MockSlackClientOptions{})
	err := main.Run(
		mockgithubclient.MakeMockGitHubClientGetter(mockgithubclient.MockGitHubClientOptions{
			ErrByPRNumber:          map[int]error{2: errors.New("tracked PR fetch failed")},
			MockStateForUpdateMode: &loadedState,
		}),
		mockslackclient.MakeSlackClientGetter(mockSlackAPI),
	)

	if err != nil {
		t.Fatalf("Expected Run to succeed despite the tracked PR fetch failure, got error: %v", err)
	}
	if mockSlackAPI.DeletedMessage.ChannelID != "" {
		t.Error("Expected the message to be kept, but DeleteMessage was called")
	}
}
