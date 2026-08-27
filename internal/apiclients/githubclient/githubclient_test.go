package githubclient_test

import (
	"testing"
	"time"

	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/google/go-github/v78/github"
	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
	"github.com/hellej/pr-slack-reminder-action/testhelpers/mockgithubclient"
)

type mockActionsService struct{}

func (m *mockActionsService) ListArtifacts(
	ctx context.Context, owner string, repo string, opts *github.ListArtifactsOptions,
) (*github.ArtifactList, *github.Response, error) {
	return &github.ArtifactList{}, nil, nil
}

func (m *mockActionsService) DownloadArtifact(
	ctx context.Context, owner string, repo string, artifactID int64, maxRedirects int,
) (
	*url.URL, *github.Response, error,
) {
	return &url.URL{}, nil, nil
}

type mockHTTPClient struct{}

func (m *mockHTTPClient) Get(url string) (*http.Response, error) {
	return &http.Response{StatusCode: 200}, nil
}

func newTestClient(opts mockgithubclient.MockGitHubClientOptions) githubclient.Client {
	return githubclient.NewClient(
		&mockHTTPClient{},
		&mockActionsService{},
		mockgithubclient.NewGraphQLTransport(opts),
	)
}

func noFilters(models.Repository) config.Filters {
	return config.Filters{}
}

func TestGetAuthenticatedClient(t *testing.T) {
	client := githubclient.GetAuthenticatedClient("test-token", "another-token")
	if client == nil {
		t.Fatal("Expected non-nil client, got nil")
	}
}

const sampleCommentBody = "Sample issue comment body"

func TestFindOneOrNoPRs(t *testing.T) {
	tests := []struct {
		name                    string
		mockPRs                 []*github.PullRequest
		mockReviews             map[int][]*github.PullRequestReview
		mockTimelineComments    map[int][]*github.IssueComment
		filters                 config.Filters
		fetchOptions            githubclient.PRFetchOptions
		expectedPRCount         int
		expectedPRNumber        int
		expectedApproverLogins  []string
		expectedCommenterLogins []string
	}{
		{
			name: "PR with approver and commenter",
			mockPRs: []*github.PullRequest{
				{
					Number:  github.Ptr(123),
					Title:   github.Ptr("Test PR"),
					Draft:   github.Ptr(false),
					HTMLURL: github.Ptr("https://github.com/owner/repo/pull/123"),
					User: &github.User{
						Login: github.Ptr("author"),
						Name:  github.Ptr("PR Author"),
					},
				},
			},
			mockReviews: map[int][]*github.PullRequestReview{
				123: {
					mockgithubclient.NewReview("approver1", "Approver One", "APPROVED"),
					mockgithubclient.NewReview("commenter1", "Commenter One", "COMMENTED"),
					mockgithubclient.NewReview("dependabot", "", "APPROVED", "Bot"),
				},
			},
			mockTimelineComments:    map[int][]*github.IssueComment{},
			expectedPRCount:         1,
			expectedApproverLogins:  []string{"approver1"},
			expectedCommenterLogins: []string{"commenter1"},
		},
		{
			name: "draft PR should be filtered out",
			mockPRs: []*github.PullRequest{
				{
					Number:  github.Ptr(124),
					Title:   github.Ptr("Draft PR"),
					Draft:   github.Ptr(true),
					HTMLURL: github.Ptr("https://github.com/owner/repo/pull/124"),
					User: &github.User{
						Login: github.Ptr("author"),
						Name:  github.Ptr("PR Author"),
					},
				},
			},
			mockReviews:             map[int][]*github.PullRequestReview{},
			mockTimelineComments:    map[int][]*github.IssueComment{},
			expectedPRCount:         0,
			expectedApproverLogins:  []string{},
			expectedCommenterLogins: []string{},
		},
		{
			name: "draft PR should be included when IncludeDrafts is on",
			mockPRs: []*github.PullRequest{
				{
					Number:  github.Ptr(124),
					Title:   github.Ptr("Draft PR"),
					Draft:   github.Ptr(true),
					HTMLURL: github.Ptr("https://github.com/owner/repo/pull/124"),
					User: &github.User{
						Login: github.Ptr("author"),
						Name:  github.Ptr("PR Author"),
					},
				},
			},
			mockReviews:             map[int][]*github.PullRequestReview{},
			mockTimelineComments:    map[int][]*github.IssueComment{},
			fetchOptions:            githubclient.PRFetchOptions{IncludeDrafts: true},
			expectedPRCount:         1,
			expectedApproverLogins:  []string{},
			expectedCommenterLogins: []string{},
		},
		{
			name: "PR with no reviews",
			mockPRs: []*github.PullRequest{
				{
					Number:  github.Ptr(125),
					Title:   github.Ptr("No Reviews PR"),
					Draft:   github.Ptr(false),
					HTMLURL: github.Ptr("https://github.com/owner/repo/pull/125"),
					User: &github.User{
						Login: github.Ptr("author"),
						Name:  github.Ptr("PR Author"),
					},
				},
			},
			mockReviews:             map[int][]*github.PullRequestReview{},
			mockTimelineComments:    map[int][]*github.IssueComment{},
			expectedPRCount:         1,
			expectedApproverLogins:  []string{},
			expectedCommenterLogins: []string{},
		},
		{
			name: "approver who also commented - should only appear in approvers",
			mockPRs: []*github.PullRequest{
				{
					Number:  github.Ptr(126),
					Title:   github.Ptr("Approver Also Comments PR"),
					Draft:   github.Ptr(false),
					HTMLURL: github.Ptr("https://github.com/owner/repo/pull/126"),
					User: &github.User{
						Login: github.Ptr("author"),
						Name:  github.Ptr("PR Author"),
					},
				},
			},
			mockReviews: map[int][]*github.PullRequestReview{
				126: {
					mockgithubclient.NewReview("reviewer1", "Reviewer One", "COMMENTED"),
					mockgithubclient.NewReview("reviewer1", "Reviewer One", "APPROVED"),
				},
			},
			mockTimelineComments:    map[int][]*github.IssueComment{},
			expectedPRCount:         1,
			expectedApproverLogins:  []string{"reviewer1"},
			expectedCommenterLogins: []string{},
		},
		{
			name: "author commenting own PR should be excluded from commenters",
			mockPRs: []*github.PullRequest{
				{
					Number:  github.Ptr(127),
					Title:   github.Ptr("Author Comments PR"),
					Draft:   github.Ptr(false),
					HTMLURL: github.Ptr("https://github.com/owner/repo/pull/127"),
					User: &github.User{
						Login: github.Ptr("pr-author"),
						Name:  github.Ptr("PR Author"),
					},
				},
			},
			mockReviews: map[int][]*github.PullRequestReview{
				127: {
					mockgithubclient.NewReview("pr-author", "PR Author", "COMMENTED"),
					mockgithubclient.NewReview("external-reviewer", "External Reviewer", "APPROVED"),
				},
			},
			mockTimelineComments:    map[int][]*github.IssueComment{},
			expectedPRCount:         1,
			expectedApproverLogins:  []string{"external-reviewer"},
			expectedCommenterLogins: []string{},
		},
		{
			name: "bot reviews should be excluded completely",
			mockPRs: []*github.PullRequest{
				{
					Number:  github.Ptr(128),
					Title:   github.Ptr("Bot Reviews PR"),
					Draft:   github.Ptr(false),
					HTMLURL: github.Ptr("https://github.com/owner/repo/pull/128"),
					User: &github.User{
						Login: github.Ptr("author"),
						Name:  github.Ptr("PR Author"),
					},
				},
			},
			mockReviews: map[int][]*github.PullRequestReview{
				128: {
					mockgithubclient.NewReview("dependabot[bot]", "", "APPROVED", "Bot"),
					mockgithubclient.NewReview("codecov[bot]", "", "COMMENTED", "Bot"),
					mockgithubclient.NewReview("human-reviewer", "Human Reviewer", "COMMENTED"),
				},
			},
			mockTimelineComments:    map[int][]*github.IssueComment{},
			expectedPRCount:         1,
			expectedApproverLogins:  []string{},
			expectedCommenterLogins: []string{"human-reviewer"},
		},
		{
			name: "invalid reviews should be excluded",
			mockPRs: []*github.PullRequest{
				{
					Number:  github.Ptr(129),
					Title:   github.Ptr("Invalid Reviews PR"),
					Draft:   github.Ptr(false),
					HTMLURL: github.Ptr("https://github.com/owner/repo/pull/129"),
					User: &github.User{
						Login: github.Ptr("author"),
						Name:  github.Ptr("PR Author"),
					},
				},
			},
			mockReviews: map[int][]*github.PullRequestReview{
				129: {
					{ // nil user review retained for invalid case
						User:  nil,
						State: github.Ptr("APPROVED"),
					},
					mockgithubclient.NewReview("", "Empty Login User", "COMMENTED"),
					mockgithubclient.NewReview("valid-reviewer", "Valid Reviewer", "APPROVED"),
				},
			},
			mockTimelineComments:    map[int][]*github.IssueComment{},
			expectedPRCount:         1,
			expectedApproverLogins:  []string{"valid-reviewer"},
			expectedCommenterLogins: []string{},
		},
		{
			name: "commenter on the diff is included through the implicit review",
			mockPRs: []*github.PullRequest{
				{
					Number:  github.Ptr(130),
					Title:   github.Ptr("PR with a bare diff comment"),
					Draft:   github.Ptr(false),
					HTMLURL: github.Ptr("https://github.com/owner/repo/pull/130"),
					User: &github.User{
						Login: github.Ptr("author"),
						Name:  github.Ptr("PR Author"),
					},
				},
			},
			// GitHub wraps a bare diff comment in a COMMENTED review with an empty body.
			mockReviews: map[int][]*github.PullRequestReview{
				130: {
					mockgithubclient.NewReview("diff-commenter", "Diff Commenter", "COMMENTED"),
					mockgithubclient.NewReview("approver", "Approver", "APPROVED"),
				},
			},
			mockTimelineComments:    map[int][]*github.IssueComment{},
			expectedPRCount:         1,
			expectedApproverLogins:  []string{"approver"},
			expectedCommenterLogins: []string{"diff-commenter"},
		},
		{
			name: "PR timeline commenters should be included in commenters",
			mockPRs: []*github.PullRequest{
				{
					Number:  github.Ptr(131),
					Title:   github.Ptr("PR with timeline comments"),
					Draft:   github.Ptr(false),
					HTMLURL: github.Ptr("https://github.com/owner/repo/pull/131"),
					User: &github.User{
						Login: github.Ptr("author"),
						Name:  github.Ptr("PR Author"),
					},
				},
			},
			mockReviews: map[int][]*github.PullRequestReview{},
			mockTimelineComments: map[int][]*github.IssueComment{
				131: {
					mockgithubclient.NewTimelineComment("issue-commenter", "Issue Commenter", sampleCommentBody, time.Time{}),
					mockgithubclient.NewTimelineComment("another-issue-commenter", "Another Issue Commenter", sampleCommentBody, time.Time{}),
					mockgithubclient.NewTimelineComment("author", "PR Author", sampleCommentBody, time.Time{}),      // should be excluded (PR author)
					mockgithubclient.NewTimelineComment("bot-commenter", "", sampleCommentBody, time.Time{}, "Bot"), // should be excluded (bot)
				},
			},
			expectedPRCount:         1,
			expectedApproverLogins:  []string{},
			expectedCommenterLogins: []string{"issue-commenter", "another-issue-commenter"},
		},
		{
			name: "PR with title containing ignored-term should be filtered out",
			mockPRs: []*github.PullRequest{
				{
					Number:  github.Ptr(132),
					Title:   github.Ptr("Release v1.0 (beta)"),
					Draft:   github.Ptr(false),
					HTMLURL: github.Ptr("https://github.com/owner/repo/pull/132"),
					User: &github.User{
						Login: github.Ptr("author"),
						Name:  github.Ptr("PR Author"),
					},
				},
				{
					Number:  github.Ptr(133),
					Title:   github.Ptr("Feature Implementation"),
					Draft:   github.Ptr(false),
					HTMLURL: github.Ptr("https://github.com/owner/repo/pull/133"),
					User: &github.User{
						Login: github.Ptr("author"),
						Name:  github.Ptr("PR Author"),
					},
				},
			},
			mockReviews:             map[int][]*github.PullRequestReview{},
			mockTimelineComments:    map[int][]*github.IssueComment{},
			filters:                 config.Filters{IgnoredTerms: []string{"Release v", "Automated Update"}},
			expectedPRCount:         1,
			expectedPRNumber:        133,
			expectedApproverLogins:  []string{},
			expectedCommenterLogins: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expectedPRCount != 0 && tt.expectedPRCount != 1 {
				t.Fatalf("Test setup error: TestFindOneOrNoPRs expects 0 or 1 PR, got expectedPRCount=%d", tt.expectedPRCount)
			}

			client := newTestClient(mockgithubclient.MockGitHubClientOptions{
				PRs:                        tt.mockPRs,
				ReviewsByPRNumber:          tt.mockReviews,
				TimelineCommentsByPRNumber: tt.mockTimelineComments,
			})

			repos := []models.Repository{
				{Owner: "testowner", Name: "testrepo"},
			}

			getFilters := func(repo models.Repository) config.Filters {
				return tt.filters
			}

			result, err := client.FindOpenPRs(context.Background(), repos, getFilters, tt.fetchOptions)

			if err != nil {
				t.Fatalf("FindOpenPRs() returned error: %v", err)
			}

			if len(result.PRs) != tt.expectedPRCount {
				t.Errorf("Expected %d PRs, got %d", tt.expectedPRCount, len(result.PRs))
				return
			}

			if tt.expectedPRCount > 0 {
				pr := result.PRs[0]

				expectedNumber := *tt.mockPRs[0].Number
				if tt.expectedPRNumber > 0 {
					expectedNumber = tt.expectedPRNumber
				}
				if pr.GetNumber() != expectedNumber {
					t.Errorf("Expected PR number %d, got %d", expectedNumber, pr.GetNumber())
				}

				var expectedPR *github.PullRequest
				if tt.expectedPRNumber > 0 {
					for _, mockPR := range tt.mockPRs {
						if *mockPR.Number == tt.expectedPRNumber {
							expectedPR = mockPR
							break
						}
					}
					if expectedPR == nil {
						t.Fatalf("Test setup error: expectedPRNumber %d not found in mockPRs", tt.expectedPRNumber)
					}
				} else {
					expectedPR = tt.mockPRs[0]
				}

				actualApproverLogins := collaboratorLogins(pr.ApprovedByUsers)
				actualCommenterLogins := collaboratorLogins(pr.CommentedByUsers)

				if !slicesEqualIgnoreOrder(tt.expectedApproverLogins, actualApproverLogins) {
					t.Errorf("Expected approver logins %v, got %v", tt.expectedApproverLogins, actualApproverLogins)
				}

				if !slicesEqualIgnoreOrder(tt.expectedCommenterLogins, actualCommenterLogins) {
					t.Errorf("Expected commenter logins %v, got %v", tt.expectedCommenterLogins, actualCommenterLogins)
				}

				expectedRepo := models.Repository{Owner: "testowner", Name: "testrepo"}
				if pr.Repository != expectedRepo {
					t.Errorf("Expected repository %v, got %v", expectedRepo, pr.Repository)
				}

				expectedAuthor := *expectedPR.User.Login
				if pr.Author.Login != expectedAuthor {
					t.Errorf("Expected author login %s, got %s", expectedAuthor, pr.Author.Login)
				}
			}
		})
	}
}

func TestFetchManyPRs(t *testing.T) {
	tests := []struct {
		prCount         int
		expectedPRCount int
	}{
		{prCount: 30, expectedPRCount: 30},
		{prCount: 50, expectedPRCount: 50},
		{prCount: 75, expectedPRCount: 50},  // maximum of 50 expected
		{prCount: 100, expectedPRCount: 50}, // maximum of 50 expected
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d PRs", tt.prCount), func(t *testing.T) {
			var mockPRs []*github.PullRequest
			for i := 1; i <= tt.prCount; i++ {
				mockPRs = append(mockPRs, &github.PullRequest{
					Number:    github.Ptr(i),
					Title:     github.Ptr(fmt.Sprintf("PR %d", i)),
					Draft:     github.Ptr(false),
					CreatedAt: &github.Timestamp{Time: time.Now().Add(-time.Duration(i) * time.Hour)},
					HTMLURL:   github.Ptr(fmt.Sprintf("https://example.com/r/%d", i)),
					User:      &github.User{Login: github.Ptr("author")},
				})
			}

			client := newTestClient(mockgithubclient.MockGitHubClientOptions{PRs: mockPRs})
			repos := []models.Repository{{Owner: "testowner", Name: "testrepo"}}

			result, err := client.FindOpenPRs(
				context.Background(), repos, noFilters, githubclient.PRFetchOptions{},
			)

			if err != nil {
				t.Fatalf("FindOpenPRs() returned error: %v", err)
			}

			if len(result.PRs) != tt.expectedPRCount {
				t.Errorf("Expected %d PRs, got %d", tt.expectedPRCount, len(result.PRs))
			}
		})
	}
}

func TestFindOpenPRs_MultipleRepositories(t *testing.T) {
	client := newTestClient(mockgithubclient.MockGitHubClientOptions{
		PRsByRepo: map[string][]*github.PullRequest{
			"repo1": {
				{
					Number:  github.Ptr(1),
					Title:   github.Ptr("Repo1 PR"),
					Draft:   github.Ptr(false),
					HTMLURL: github.Ptr("https://example.com/r1/1"),
					User:    &github.User{Login: github.Ptr("author1")},
				},
			},
			"repo2": {
				{
					Number:  github.Ptr(2),
					Title:   github.Ptr("Repo2 PR"),
					Draft:   github.Ptr(false),
					HTMLURL: github.Ptr("https://example.com/r2/2"),
					User:    &github.User{Login: github.Ptr("author2")},
				},
			},
		},
	})

	repos := []models.Repository{{Owner: "o", Name: "repo1"}, {Owner: "o", Name: "repo2"}}
	result, err := client.FindOpenPRs(context.Background(), repos, noFilters, githubclient.PRFetchOptions{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.PRs) != 2 {
		t.Fatalf("expected 2 PRs, got %d", len(result.PRs))
	}
	numbers := []int{result.PRs[0].GetNumber(), result.PRs[1].GetNumber()}
	if !((numbers[0] == 1 && numbers[1] == 2) || (numbers[0] == 2 && numbers[1] == 1)) {
		t.Errorf("expected PR numbers 1 and 2, got %v", numbers)
	}
}

func TestFindOpenPRs_ErrorShortCircuits(t *testing.T) {
	client := newTestClient(mockgithubclient.MockGitHubClientOptions{
		PRsByRepo: map[string][]*github.PullRequest{
			"good": {
				{
					Number:  github.Ptr(3),
					Title:   github.Ptr("Ok PR"),
					Draft:   github.Ptr(false),
					HTMLURL: github.Ptr("https://example.com/r/3"),
					User:    &github.User{Login: github.Ptr("author")},
				},
			},
		},
		ListPRsResponseStatus: 404, // "bad" has no fixture, so its alias comes back NOT_FOUND
		PRServiceError:        errors.New("not found"),
	})

	repos := []models.Repository{{Owner: "o", Name: "bad"}, {Owner: "o", Name: "good"}}
	_, err := client.FindOpenPRs(context.Background(), repos, noFilters, githubclient.PRFetchOptions{})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestFindOpenPRs_EnrichmentIsBatched(t *testing.T) {
	prCount := 30 // more than the 25 aliases one enrichment request carries
	mockPRs := make([]*github.PullRequest, 0, prCount)
	for i := range prCount {
		mockPRs = append(mockPRs, &github.PullRequest{
			Number:    github.Ptr(i + 100),
			Title:     github.Ptr(fmt.Sprintf("PR %d", i)),
			Draft:     github.Ptr(false),
			CreatedAt: &github.Timestamp{Time: time.Now().Add(-time.Duration(i) * time.Hour)},
			HTMLURL:   github.Ptr(fmt.Sprintf("https://example.com/r/%d", i)),
			User:      &github.User{Login: github.Ptr("author")},
		})
	}

	client := newTestClient(mockgithubclient.MockGitHubClientOptions{PRs: mockPRs})
	repos := []models.Repository{{Owner: "o", Name: "repo"}}

	result, err := client.FindOpenPRs(context.Background(), repos, noFilters, githubclient.PRFetchOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.PRs) != prCount {
		t.Fatalf("expected %d PRs, got %d", prCount, len(result.PRs))
	}
}

func TestFindOpenPRs_ReviewsPartialErrors(t *testing.T) {
	// Two PRs: enriching the first fails, the second succeeds.
	client := newTestClient(mockgithubclient.MockGitHubClientOptions{
		PRs: []*github.PullRequest{
			{Number: github.Ptr(101), Title: github.Ptr("PR One"), Draft: github.Ptr(false), HTMLURL: github.Ptr("https://example.com/repo/101"), User: &github.User{Login: github.Ptr("author1")}},
			{Number: github.Ptr(102), Title: github.Ptr("PR Two"), Draft: github.Ptr(false), HTMLURL: github.Ptr("https://example.com/repo/102"), User: &github.User{Login: github.Ptr("author2")}},
		},
		ReviewsByPRNumber: map[int][]*github.PullRequestReview{
			102: { // success case only
				mockgithubclient.NewReview("approver2", "Approver Two", "APPROVED"),
				mockgithubclient.NewReview("commenter2", "Commenter Two", "COMMENTED"),
			},
		},
		ErrByPRNumber: map[int]error{
			101: fmt.Errorf("network timeout"), // failure for first PR
		},
	})

	repos := []models.Repository{{Owner: "o", Name: "repo"}}
	result, err := client.FindOpenPRs(context.Background(), repos, noFilters, githubclient.PRFetchOptions{})
	if err != nil {
		t.Fatalf("did not expect error, got %v", err)
	}
	prs := result.PRs
	if len(prs) != 2 {
		t.Fatalf("expected 2 PRs, got %d", len(prs))
	}
	var pr1, pr2 *githubclient.PR
	for i := range prs {
		switch prs[i].GetNumber() {
		case 101:
			pr1 = &prs[i]
		case 102:
			pr2 = &prs[i]
		}
	}
	if pr1 == nil || pr2 == nil {
		t.Fatalf("missing expected PR numbers; got: %v,%v", pr1, pr2)
	}
	// PR1 had review fetch error, should have no reviewers/commenters
	if len(pr1.ApprovedByUsers) != 0 || len(pr1.CommentedByUsers) != 0 {
		t.Errorf("expected PR1 to have no reviewer info due to error, got approvers=%d commenters=%d", len(pr1.ApprovedByUsers), len(pr1.CommentedByUsers))
	}
	// PR2 had events -> one approver and one commenter
	if len(pr2.ApprovedByUsers) != 1 || pr2.ApprovedByUsers[0].Login != "approver2" {
		t.Errorf("expected PR2 approver 'approver2', got %+v", pr2.ApprovedByUsers)
	}
	if len(pr2.CommentedByUsers) != 1 || pr2.CommentedByUsers[0].Login != "commenter2" {
		t.Errorf("expected PR2 commenter 'commenter2', got %+v", pr2.CommentedByUsers)
	}
}

func TestGetPRsMapsStateAndMerged(t *testing.T) {
	tests := []struct {
		name           string
		state          string
		merged         bool
		expectedState  string
		expectedMerged bool
	}{
		{name: "open PR", state: "open", expectedState: "open"},
		{name: "merged PR", state: "closed", merged: true, expectedState: "closed", expectedMerged: true},
		{name: "closed PR without merge", state: "closed", expectedState: "closed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(mockgithubclient.MockGitHubClientOptions{
				PRsByNumber: map[int]*github.PullRequest{
					7: {
						Number:  github.Ptr(7),
						Title:   github.Ptr("Fetched PR"),
						Draft:   github.Ptr(false),
						HTMLURL: github.Ptr("https://example.com/repo/7"),
						State:   github.Ptr(tt.state),
						Merged:  github.Ptr(tt.merged),
						User:    &github.User{Login: github.Ptr("author"), Name: github.Ptr("PR Author")},
					},
				},
				ReviewsByPRNumber: map[int][]*github.PullRequestReview{
					7: {mockgithubclient.NewReview("approver1", "Approver One", "APPROVED")},
				},
			})

			references := []models.PullRequestRef{
				{Repository: models.Repository{Owner: "testowner", Name: "testrepo"}, Number: 7},
			}
			prs, err := client.GetPRs(context.Background(), references, noFilters)

			if err != nil {
				t.Fatalf("GetPRs() returned error: %v", err)
			}
			if len(prs) != 1 {
				t.Fatalf("expected 1 PR, got %d", len(prs))
			}
			pr := prs[0]
			if pr.GetState() != tt.expectedState {
				t.Errorf("expected state %q, got %q", tt.expectedState, pr.GetState())
			}
			if pr.GetMerged() != tt.expectedMerged {
				t.Errorf("expected merged %t, got %t", tt.expectedMerged, pr.GetMerged())
			}
			if pr.GetTitle() != "Fetched PR" || pr.GetHTMLURL() != "https://example.com/repo/7" {
				t.Errorf("unexpected scalars: title %q, url %q", pr.GetTitle(), pr.GetHTMLURL())
			}
			if pr.Author.Login != "author" || pr.Author.Name != "PR Author" {
				t.Errorf("unexpected author: %+v", pr.Author)
			}
			if !slicesEqualIgnoreOrder([]string{"approver1"}, collaboratorLogins(pr.ApprovedByUsers)) {
				t.Errorf("expected approver 'approver1', got %+v", pr.ApprovedByUsers)
			}
		})
	}
}

func TestGetPRsFailsWhenPRIsNotFound(t *testing.T) {
	client := newTestClient(mockgithubclient.MockGitHubClientOptions{})

	references := []models.PullRequestRef{
		{Repository: models.Repository{Owner: "test-org", Name: "test-repo"}, Number: 404},
	}
	_, err := client.GetPRs(context.Background(), references, noFilters)

	expectedMessage := "PR test-org/test-repo/404 not found - check the path and permissions"
	if err == nil {
		t.Fatalf("expected error %q, got nil", expectedMessage)
	}
	if !strings.Contains(err.Error(), expectedMessage) {
		t.Errorf("expected error message %q, got: %v", expectedMessage, err)
	}
}

func collaboratorLogins(collaborators []githubclient.Collaborator) []string {
	return utilities.Map(collaborators, func(c githubclient.Collaborator) string { return c.Login })
}

func slicesEqualIgnoreOrder(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	mapA := make(map[string]bool)
	for _, v := range a {
		mapA[v] = true
	}
	for _, v := range b {
		if !mapA[v] {
			return false
		}
	}
	return true
}

func makePRFixture(number int, draft bool, createdAt, updatedAt time.Time) *github.PullRequest {
	return &github.PullRequest{
		Number:    github.Ptr(number),
		Title:     github.Ptr(fmt.Sprintf("PR %d", number)),
		Draft:     github.Ptr(draft),
		CreatedAt: &github.Timestamp{Time: createdAt},
		UpdatedAt: &github.Timestamp{Time: updatedAt},
		HTMLURL:   github.Ptr(fmt.Sprintf("https://example.com/r/%d", number)),
		User:      &github.User{Login: github.Ptr("author")},
	}
}

// Numbers run from firstNumber+1 upwards, newest created first.
func makeOpenPRFixtures(count, firstNumber int) []*github.PullRequest {
	prs := make([]*github.PullRequest, 0, count)
	for i := 1; i <= count; i++ {
		createdAt := time.Now().Add(-time.Duration(i) * time.Hour)
		prs = append(prs, makePRFixture(firstNumber+i, false, createdAt, createdAt))
	}
	return prs
}

// Numbers run from firstNumber+1 upwards, newest updated first but oldest created first, so a
// creation-date cap would keep a different set than the update-date cap drafts are capped by.
func makeDraftPRFixtures(count, firstNumber int) []*github.PullRequest {
	prs := make([]*github.PullRequest, 0, count)
	for i := 1; i <= count; i++ {
		createdAt := time.Now().Add(-time.Duration(count-i) * time.Hour)
		updatedAt := time.Now().Add(-time.Duration(i) * time.Hour)
		prs = append(prs, makePRFixture(firstNumber+i, true, createdAt, updatedAt))
	}
	return prs
}

func prNumbers(prs []githubclient.PR) []int {
	numbers := utilities.Map(prs, func(pr githubclient.PR) int { return pr.GetNumber() })
	slices.Sort(numbers)
	return numbers
}

func TestFindOpenPRs_CapsDraftsAndOpenPRsSeparately(t *testing.T) {
	const draftFirstNumber = 1000

	tests := []struct {
		name                   string
		openPRCount            int
		draftPRCount           int
		includeDrafts          bool
		expectedOpenPRNumbers  []int
		expectedDraftPRNumbers []int
		expectedOpenPRsCapped  bool
		expectedDraftPRsCapped bool
	}{
		{
			name:                  "drafts excluded, open PRs capped at MaxPRsToFetch",
			openPRCount:           60,
			draftPRCount:          20,
			includeDrafts:         false,
			expectedOpenPRNumbers: rangeOfNumbers(1, githubclient.MaxPRsToFetch),
			expectedOpenPRsCapped: true,
		},
		{
			name:                   "both buckets over their caps",
			openPRCount:            60,
			draftPRCount:           20,
			includeDrafts:          true,
			expectedOpenPRNumbers:  rangeOfNumbers(1, githubclient.MaxPRsToFetch),
			expectedDraftPRNumbers: rangeOfNumbers(draftFirstNumber+1, 15), // MaxDraftPRsToFetch, kept literal on purpose
			expectedOpenPRsCapped:  true,
			expectedDraftPRsCapped: true,
		},
		{
			name:                   "only the draft bucket overflows",
			openPRCount:            3,
			draftPRCount:           20,
			includeDrafts:          true,
			expectedOpenPRNumbers:  rangeOfNumbers(1, 3),
			expectedDraftPRNumbers: rangeOfNumbers(draftFirstNumber+1, 15), // MaxDraftPRsToFetch, kept literal on purpose
			expectedDraftPRsCapped: true,
		},
		{
			name:                   "only the open bucket overflows",
			openPRCount:            60,
			draftPRCount:           5,
			includeDrafts:          true,
			expectedOpenPRNumbers:  rangeOfNumbers(1, githubclient.MaxPRsToFetch),
			expectedDraftPRNumbers: rangeOfNumbers(draftFirstNumber+1, 5),
			expectedOpenPRsCapped:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPRs := slices.Concat(
				makeOpenPRFixtures(tt.openPRCount, 0),
				makeDraftPRFixtures(tt.draftPRCount, draftFirstNumber),
			)
			client := newTestClient(mockgithubclient.MockGitHubClientOptions{PRs: mockPRs})
			repos := []models.Repository{{Owner: "o", Name: "repo"}}

			result, err := client.FindOpenPRs(
				context.Background(),
				repos,
				noFilters,
				githubclient.PRFetchOptions{IncludeDrafts: tt.includeDrafts},
			)
			if err != nil {
				t.Fatalf("FindOpenPRs() returned error: %v", err)
			}

			openPRs := utilities.Filter(result.PRs, func(pr githubclient.PR) bool { return !pr.GetDraft() })
			draftPRs := utilities.Filter(result.PRs, func(pr githubclient.PR) bool { return pr.GetDraft() })

			if !slices.Equal(prNumbers(openPRs), tt.expectedOpenPRNumbers) {
				t.Errorf("expected open PR numbers %v, got %v", tt.expectedOpenPRNumbers, prNumbers(openPRs))
			}
			if !slices.Equal(prNumbers(draftPRs), tt.expectedDraftPRNumbers) {
				t.Errorf("expected draft PR numbers %v, got %v", tt.expectedDraftPRNumbers, prNumbers(draftPRs))
			}
			if result.OpenPRsCapped != tt.expectedOpenPRsCapped {
				t.Errorf("expected OpenPRsCapped %v, got %v", tt.expectedOpenPRsCapped, result.OpenPRsCapped)
			}
			if result.DraftPRsCapped != tt.expectedDraftPRsCapped {
				t.Errorf("expected DraftPRsCapped %v, got %v", tt.expectedDraftPRsCapped, result.DraftPRsCapped)
			}
		})
	}
}

func rangeOfNumbers(first, count int) []int {
	numbers := make([]int, 0, count)
	for i := range count {
		numbers = append(numbers, first+i)
	}
	return numbers
}

// The activity timestamp the canvas reads is the PR's own update time, carried through
// enrichment untouched.
func TestFindOpenPRsCarriesTheUpdateTimeThroughEnrichment(t *testing.T) {
	createdAt := time.Now().Add(-40 * time.Hour).UTC().Truncate(time.Second)
	updatedAt := time.Now().Add(-9 * time.Hour).UTC().Truncate(time.Second)

	client := newTestClient(mockgithubclient.MockGitHubClientOptions{
		PRs: []*github.PullRequest{makePRFixture(7, false, createdAt, updatedAt)},
	})

	result, err := client.FindOpenPRs(
		context.Background(),
		[]models.Repository{{Owner: "o", Name: "repo"}},
		noFilters,
		githubclient.PRFetchOptions{},
	)
	if err != nil {
		t.Fatalf("FindOpenPRs() returned error: %v", err)
	}
	if len(result.PRs) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(result.PRs))
	}
	if got := result.PRs[0].GetUpdatedAt(); !got.Equal(updatedAt) {
		t.Errorf("expected UpdatedAt %v, got %v", updatedAt, got)
	}
}
