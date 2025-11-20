package githubclient_test

import (
	"testing"
	"time"

	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/google/go-github/v78/github"
	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/internal/models"
)

type mockPullRequestService struct {
	mockPRs                []*github.PullRequest
	mockPRsByNumber        map[int]*github.PullRequest
	mockReviewsByPRNumber  map[int][]*github.PullRequestReview
	mockCommentsByPRNumber map[int][]*github.PullRequestComment
	mockResponse           *github.Response
	mockError              error
}

func (m *mockPullRequestService) Get(
	ctx context.Context, owner string, repo string, number int,
) (*github.PullRequest, *github.Response, error) {
	if pr, ok := m.mockPRsByNumber[number]; ok {
		return pr, m.mockResponse, m.mockError
	}
	return nil, &github.Response{Response: &http.Response{StatusCode: 404}}, fmt.Errorf("unknown repo")
}

func (m *mockPullRequestService) List(
	ctx context.Context, owner string, repo string, opts *github.PullRequestListOptions,
) ([]*github.PullRequest, *github.Response, error) {
	return m.mockPRs, m.mockResponse, m.mockError
}

func (m *mockPullRequestService) ListReviews(
	ctx context.Context, owner string, repo string, number int, opts *github.ListOptions,
) ([]*github.PullRequestReview, *github.Response, error) {
	reviews := m.mockReviewsByPRNumber[number]
	return reviews, m.mockResponse, m.mockError
}

func (m *mockPullRequestService) ListComments(
	ctx context.Context, owner string, repo string, number int, opts *github.PullRequestListCommentsOptions,
) ([]*github.PullRequestComment, *github.Response, error) {
	comments := m.mockCommentsByPRNumber[number]
	return comments, m.mockResponse, m.mockError
}

type mockIssueService struct {
	mockTimelineCommentsByPRNumber map[int][]*github.IssueComment
	mockResponse                   *github.Response
	mockError                      error
}

func (m *mockIssueService) ListComments(
	ctx context.Context, owner string, repo string, number int, opts *github.IssueListCommentsOptions,
) ([]*github.IssueComment, *github.Response, error) {
	comments := m.mockTimelineCommentsByPRNumber[number]
	return comments, m.mockResponse, m.mockError
}

type mockActionsService struct {
	mockResponse *github.Response
	mockError    error
}

func (m *mockActionsService) ListArtifacts(
	ctx context.Context, owner string, repo string, opts *github.ListArtifactsOptions,
) (*github.ArtifactList, *github.Response, error) {
	return &github.ArtifactList{}, m.mockResponse, m.mockError
}
func (m *mockActionsService) DownloadArtifact(
	ctx context.Context, owner string, repo string, artifactID int64, maxRedirects int,
) (
	*url.URL, *github.Response, error,
) {
	return &url.URL{}, m.mockResponse, m.mockError
}

type mockHTTPClient struct {
	mockResponse *http.Response
	mockError    error
}

func (m *mockHTTPClient) Get(url string) (*http.Response, error) {
	return m.mockResponse, m.mockError
}

func NewReview(login, name, state string, userType ...string) *github.PullRequestReview {
	var t *string
	if len(userType) > 0 && userType[0] != "" {
		t = github.Ptr(userType[0])
	}
	return &github.PullRequestReview{
		User: &github.User{
			Login: github.Ptr(login),
			Name:  github.Ptr(name),
			Type:  t,
		},
		State: github.Ptr(state),
	}
}

func NewComment(login, name string, userType ...string) *github.PullRequestComment {
	var t *string
	if len(userType) > 0 && userType[0] != "" {
		t = github.Ptr(userType[0])
	}
	return &github.PullRequestComment{
		User: &github.User{
			Login: github.Ptr(login),
			Name:  github.Ptr(name),
			Type:  t,
		},
		Body: github.Ptr("Sample comment body"),
	}
}

func NewTimelineComment(login, name string, userType ...string) *github.IssueComment {
	var t *string
	if len(userType) > 0 && userType[0] != "" {
		t = github.Ptr(userType[0])
	}
	return &github.IssueComment{
		User: &github.User{
			Login: github.Ptr(login),
			Name:  github.Ptr(name),
			Type:  t,
		},
		Body: github.Ptr("Sample issue comment body"),
	}
}

// multiRepoPRService routes Get & List calls to different mock services based on repo name
type multiRepoPRService struct {
	services map[string]*mockPullRequestService
}

func (m *multiRepoPRService) Get(
	ctx context.Context, owner string, repo string, number int,
) (*github.PullRequest, *github.Response, error) {
	if svc, ok := m.services[repo]; ok {
		return svc.mockPRs[number], svc.mockResponse, svc.mockError
	}
	return nil, &github.Response{Response: &http.Response{StatusCode: 404}}, fmt.Errorf("unknown repo")
}

func (m *multiRepoPRService) List(
	ctx context.Context, owner string, repo string, opts *github.PullRequestListOptions,
) ([]*github.PullRequest, *github.Response, error) {
	if svc, ok := m.services[repo]; ok {
		return svc.mockPRs, svc.mockResponse, svc.mockError
	}
	return nil, &github.Response{Response: &http.Response{StatusCode: 404}}, fmt.Errorf("unknown repo")
}

func (m *multiRepoPRService) ListReviews(
	ctx context.Context, owner string, repo string, number int, opts *github.ListOptions,
) ([]*github.PullRequestReview, *github.Response, error) {
	if svc, ok := m.services[repo]; ok {
		reviews := svc.mockReviewsByPRNumber[number]
		return reviews, svc.mockResponse, svc.mockError
	}
	return nil, &github.Response{Response: &http.Response{StatusCode: 404}}, fmt.Errorf("unknown repo")
}

func (m *multiRepoPRService) ListComments(
	ctx context.Context, owner string, repo string, number int, opts *github.PullRequestListCommentsOptions,
) ([]*github.PullRequestComment, *github.Response, error) {
	if svc, ok := m.services[repo]; ok {
		comments := svc.mockCommentsByPRNumber[number]
		return comments, svc.mockResponse, svc.mockError
	}
	return nil, &github.Response{Response: &http.Response{StatusCode: 404}}, fmt.Errorf("unknown repo")
}

// multiRepoIssuesService routes ListComments calls to different mock services based on repo name
type multiRepoIssuesService struct {
	services map[string]*mockIssueService
}

func (m *multiRepoIssuesService) ListComments(
	ctx context.Context, owner string, repo string, number int, opts *github.IssueListCommentsOptions,
) ([]*github.IssueComment, *github.Response, error) {
	if svc, ok := m.services[repo]; ok {
		comments := svc.mockTimelineCommentsByPRNumber[number]
		return comments, svc.mockResponse, svc.mockError
	}
	return nil, &github.Response{Response: &http.Response{StatusCode: 404}}, fmt.Errorf("unknown repo")
}

func TestGetAuthenticatedClient(t *testing.T) {
	client := githubclient.GetAuthenticatedClient("test-token")
	if client == nil {
		t.Fatal("Expected non-nil client, got nil")
	}
}

func TestFindOpenPRs(t *testing.T) {
	tests := []struct {
		name                    string
		mockPRs                 []*github.PullRequest
		mockReviews             map[int][]*github.PullRequestReview
		mockComments            map[int][]*github.PullRequestComment
		mockTimelineComments    map[int][]*github.IssueComment
		filters                 config.Filters
		expectedPRCount         int
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
					NewReview("approver1", "Approver One", "APPROVED"),
					NewReview("commenter1", "Commenter One", "COMMENTED"),
					NewReview("dependabot", "", "APPROVED", "Bot"),
				},
			},
			mockComments:            map[int][]*github.PullRequestComment{},
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
			mockComments:            map[int][]*github.PullRequestComment{},
			mockTimelineComments:    map[int][]*github.IssueComment{},
			expectedPRCount:         0,
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
			mockComments:            map[int][]*github.PullRequestComment{},
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
					NewReview("reviewer1", "Reviewer One", "COMMENTED"),
					NewReview("reviewer1", "Reviewer One", "APPROVED"),
				},
			},
			mockComments:            map[int][]*github.PullRequestComment{},
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
					NewReview("pr-author", "PR Author", "COMMENTED"),
					NewReview("external-reviewer", "External Reviewer", "APPROVED"),
				},
			},
			mockComments:            map[int][]*github.PullRequestComment{},
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
					NewReview("dependabot[bot]", "", "APPROVED", "Bot"),
					NewReview("codecov[bot]", "", "COMMENTED", "Bot"),
					NewReview("human-reviewer", "Human Reviewer", "COMMENTED"),
				},
			},
			mockComments:            map[int][]*github.PullRequestComment{},
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
					NewReview("", "Empty Login User", "COMMENTED"),
					NewReview("valid-reviewer", "Valid Reviewer", "APPROVED"),
				},
			},
			mockComments:            map[int][]*github.PullRequestComment{},
			mockTimelineComments:    map[int][]*github.IssueComment{},
			expectedPRCount:         1,
			expectedApproverLogins:  []string{"valid-reviewer"},
			expectedCommenterLogins: []string{},
		},
		{
			name: "PR with both review comments and standalone comments",
			mockPRs: []*github.PullRequest{
				{
					Number:  github.Ptr(130),
					Title:   github.Ptr("PR with Mixed Comments"),
					Draft:   github.Ptr(false),
					HTMLURL: github.Ptr("https://github.com/owner/repo/pull/130"),
					User: &github.User{
						Login: github.Ptr("author"),
						Name:  github.Ptr("PR Author"),
					},
				},
			},
			mockReviews: map[int][]*github.PullRequestReview{
				130: {
					NewReview("review-commenter", "Review Commenter", "COMMENTED"),
					NewReview("approver", "Approver", "APPROVED"),
				},
			},
			mockComments: map[int][]*github.PullRequestComment{
				130: {
					NewComment("standalone-commenter", "Standalone Commenter"),
					NewComment("author", "PR Author"),  // author should be excluded
					NewComment("bot-user", "", "Bot"),  // bot should be excluded
					NewComment("approver", "Approver"), // should only appear in approvers, not commenters
				},
			},
			mockTimelineComments:    map[int][]*github.IssueComment{},
			expectedPRCount:         1,
			expectedApproverLogins:  []string{"approver"},
			expectedCommenterLogins: []string{"review-commenter", "standalone-commenter"},
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
			mockReviews:  map[int][]*github.PullRequestReview{},
			mockComments: map[int][]*github.PullRequestComment{},
			mockTimelineComments: map[int][]*github.IssueComment{
				131: {
					NewTimelineComment("issue-commenter", "Issue Commenter"),
					NewTimelineComment("another-issue-commenter", "Another Issue Commenter"),
					NewTimelineComment("author", "PR Author"),      // should be excluded (PR author)
					NewTimelineComment("bot-commenter", "", "Bot"), // should be excluded (bot)
				},
			},
			expectedPRCount:         1,
			expectedApproverLogins:  []string{},
			expectedCommenterLogins: []string{"issue-commenter", "another-issue-commenter"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPRService := &mockPullRequestService{
				mockPRs:                tt.mockPRs,
				mockReviewsByPRNumber:  tt.mockReviews,
				mockCommentsByPRNumber: tt.mockComments,
				mockResponse: &github.Response{
					Response: &http.Response{
						StatusCode: 200,
					},
				},
				mockError: nil,
			}
			mockIssueService := &mockIssueService{
				mockTimelineCommentsByPRNumber: tt.mockTimelineComments,
				mockResponse:                   &github.Response{Response: &http.Response{StatusCode: 200}},
				mockError:                      nil,
			}
			mockActionsService := &mockActionsService{
				mockResponse: &github.Response{Response: &http.Response{StatusCode: 200}},
				mockError:    nil,
			}
			mockHTTPClient := &mockHTTPClient{
				mockResponse: &http.Response{StatusCode: 200},
				mockError:    nil,
			}
			client := githubclient.NewClient(mockHTTPClient, mockPRService, mockIssueService, mockActionsService)

			repos := []models.Repository{
				{Owner: "testowner", Name: "testrepo"},
			}

			getFilters := func(repo models.Repository) config.Filters {
				return config.Filters{} // empty filters = allow all
			}

			result, err := client.FindOpenPRs(context.Background(), repos, getFilters)

			if err != nil {
				t.Fatalf("FindOpenPRs() returned error: %v", err)
			}

			if len(result) != tt.expectedPRCount {
				t.Errorf("Expected %d PRs, got %d", tt.expectedPRCount, len(result))
				return
			}

			if tt.expectedPRCount > 0 {
				pr := result[0]

				if pr.GetNumber() != *tt.mockPRs[0].Number {
					t.Errorf("Expected PR number %d, got %d", *tt.mockPRs[0].Number, pr.GetNumber())
				}

				actualApproverLogins := make([]string, len(pr.ApprovedByUsers))
				for i, user := range pr.ApprovedByUsers {
					actualApproverLogins[i] = user.Login
				}

				actualCommenterLogins := make([]string, len(pr.CommentedByUsers))
				for i, user := range pr.CommentedByUsers {
					actualCommenterLogins[i] = user.Login
				}

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

				expectedAuthor := *tt.mockPRs[0].User.Login
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

			mockPRService := &mockPullRequestService{
				mockPRs:                mockPRs,
				mockReviewsByPRNumber:  map[int][]*github.PullRequestReview{},
				mockCommentsByPRNumber: map[int][]*github.PullRequestComment{},
				mockResponse:           &github.Response{Response: &http.Response{StatusCode: 200}},
				mockError:              nil,
			}
			mockIssueService := &mockIssueService{
				mockTimelineCommentsByPRNumber: map[int][]*github.IssueComment{},
				mockResponse:                   &github.Response{Response: &http.Response{StatusCode: 200}},
				mockError:                      nil,
			}
			mockHTTPClient := &mockHTTPClient{
				mockResponse: &http.Response{StatusCode: 200},
				mockError:    nil,
			}
			mockActionsService := &mockActionsService{
				mockResponse: &github.Response{Response: &http.Response{StatusCode: 200}},
				mockError:    nil,
			}
			client := githubclient.NewClient(mockHTTPClient, mockPRService, mockIssueService, mockActionsService)
			repos := []models.Repository{{Owner: "testowner", Name: "testrepo"}}

			result, err := client.FindOpenPRs(
				context.Background(),
				repos, func(models.Repository) config.Filters {
					return config.Filters{}
				},
			)

			if err != nil {
				t.Fatalf("FindOpenPRs() returned error: %v", err)
			}

			if len(result) != tt.expectedPRCount {
				t.Errorf("Expected %d PRs, got %d", tt.expectedPRCount, len(result))
			}
		})
	}
}

func TestFindOpenPRs_MultipleRepositories(t *testing.T) {
	mockPRService1 := &mockPullRequestService{
		mockPRs: []*github.PullRequest{
			{
				Number:  github.Ptr(1),
				Title:   github.Ptr("Repo1 PR"),
				Draft:   github.Ptr(false),
				HTMLURL: github.Ptr("https://example.com/r1/1"),
				User:    &github.User{Login: github.Ptr("author1")},
			},
		},
		mockReviewsByPRNumber:  map[int][]*github.PullRequestReview{},
		mockCommentsByPRNumber: map[int][]*github.PullRequestComment{},
		mockResponse:           &github.Response{Response: &http.Response{StatusCode: 200}},
		mockError:              nil,
	}
	mockPRService2 := &mockPullRequestService{
		mockPRs: []*github.PullRequest{
			{
				Number:  github.Ptr(2),
				Title:   github.Ptr("Repo2 PR"),
				Draft:   github.Ptr(false),
				HTMLURL: github.Ptr("https://example.com/r2/2"),
				User:    &github.User{Login: github.Ptr("author2")},
			},
		},
		mockReviewsByPRNumber:  map[int][]*github.PullRequestReview{},
		mockCommentsByPRNumber: map[int][]*github.PullRequestComment{},
		mockResponse:           &github.Response{Response: &http.Response{StatusCode: 200}},
		mockError:              nil,
	}

	mockIssueService1 := &mockIssueService{
		mockTimelineCommentsByPRNumber: map[int][]*github.IssueComment{},
		mockResponse:                   &github.Response{Response: &http.Response{StatusCode: 200}},
		mockError:                      nil,
	}
	mockIssueService2 := &mockIssueService{
		mockTimelineCommentsByPRNumber: map[int][]*github.IssueComment{},
		mockResponse:                   &github.Response{Response: &http.Response{StatusCode: 200}},
		mockError:                      nil,
	}
	mockHTTPClient := &mockHTTPClient{
		mockResponse: &http.Response{StatusCode: 200},
		mockError:    nil,
	}
	mockActionsService := &mockActionsService{
		mockResponse: &github.Response{Response: &http.Response{StatusCode: 200}},
		mockError:    nil,
	}
	client := githubclient.NewClient(
		mockHTTPClient,
		&multiRepoPRService{
			services: map[string]*mockPullRequestService{"repo1": mockPRService1, "repo2": mockPRService2},
		},
		&multiRepoIssuesService{
			services: map[string]*mockIssueService{"repo1": mockIssueService1, "repo2": mockIssueService2},
		},
		mockActionsService,
	)
	repos := []models.Repository{{Owner: "o", Name: "repo1"}, {Owner: "o", Name: "repo2"}}
	result, err := client.FindOpenPRs(
		context.Background(),
		repos, func(models.Repository) config.Filters {
			return config.Filters{}
		},
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 PRs, got %d", len(result))
	}
	numbers := []int{result[0].GetNumber(), result[1].GetNumber()}
	if !((numbers[0] == 1 && numbers[1] == 2) || (numbers[0] == 2 && numbers[1] == 1)) {
		t.Errorf("expected PR numbers 1 and 2, got %v", numbers)
	}
}

func TestFindOpenPRs_ErrorShortCircuits(t *testing.T) {
	mockPRService404 := &mockPullRequestService{
		mockPRs: nil, mockReviewsByPRNumber: map[int][]*github.PullRequestReview{},
		mockCommentsByPRNumber: map[int][]*github.PullRequestComment{},
		mockResponse:           &github.Response{Response: &http.Response{StatusCode: 404}},
		mockError:              fmt.Errorf("not found"),
	}
	mockPRServiceOK := &mockPullRequestService{
		mockPRs: []*github.PullRequest{
			{
				Number:  github.Ptr(3),
				Title:   github.Ptr("Ok PR"),
				Draft:   github.Ptr(false),
				HTMLURL: github.Ptr("https://example.com/r/3"),
				User:    &github.User{Login: github.Ptr("author")},
			},
		},
		mockReviewsByPRNumber:  map[int][]*github.PullRequestReview{},
		mockCommentsByPRNumber: map[int][]*github.PullRequestComment{},
		mockResponse:           &github.Response{Response: &http.Response{StatusCode: 200}},
		mockError:              nil,
	}

	mockHTTPClient := &mockHTTPClient{
		mockResponse: &http.Response{StatusCode: 200},
		mockError:    nil,
	}
	mockActionsService := &mockActionsService{
		mockResponse: &github.Response{Response: &http.Response{StatusCode: 200}},
		mockError:    nil,
	}
	client := githubclient.NewClient(
		mockHTTPClient,
		&multiRepoPRService{
			services: map[string]*mockPullRequestService{"bad": mockPRService404, "good": mockPRServiceOK},
		},
		&multiRepoIssuesService{
			services: map[string]*mockIssueService{
				"bad": {
					mockTimelineCommentsByPRNumber: map[int][]*github.IssueComment{},
					mockResponse:                   &github.Response{Response: &http.Response{StatusCode: 404}},
					mockError:                      fmt.Errorf("not found"),
				},
				"good": {
					mockTimelineCommentsByPRNumber: map[int][]*github.IssueComment{},
					mockResponse:                   &github.Response{Response: &http.Response{StatusCode: 200}},
					mockError:                      nil,
				},
			},
		},
		mockActionsService,
	)
	repos := []models.Repository{{Owner: "o", Name: "bad"}, {Owner: "o", Name: "good"}}
	_, err := client.FindOpenPRs(
		context.Background(),
		repos,
		func(models.Repository) config.Filters { return config.Filters{} },
	)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestFindOpenPRs_ConcurrencyLimit(t *testing.T) {
	repoCount := githubclient.DefaultGitHubAPIConcurrencyLimit + 3
	services := make(map[string]*mockPullRequestService)
	repos := make([]models.Repository, 0, repoCount)
	for i := 0; i < repoCount; i++ {
		name := fmt.Sprintf("repo-%d", i)
		services[name] = &mockPullRequestService{
			mockPRs: []*github.PullRequest{
				{
					Number:  github.Ptr(i + 100),
					Title:   github.Ptr(name + " PR"),
					Draft:   github.Ptr(false),
					HTMLURL: github.Ptr("https://example.com/" + name),
					User:    &github.User{Login: github.Ptr("author")}},
			},
			mockReviewsByPRNumber:  map[int][]*github.PullRequestReview{},
			mockCommentsByPRNumber: map[int][]*github.PullRequestComment{},
			mockResponse:           &github.Response{Response: &http.Response{StatusCode: 200}},
			mockError:              nil,
		}
		repos = append(repos, models.Repository{Owner: "o", Name: name})
	}
	issueServices := make(map[string]*mockIssueService)
	for name := range services {
		issueServices[name] = &mockIssueService{
			mockTimelineCommentsByPRNumber: map[int][]*github.IssueComment{},
			mockResponse:                   &github.Response{Response: &http.Response{StatusCode: 200}},
			mockError:                      nil,
		}
	}
	mockHTTPClient := &mockHTTPClient{
		mockResponse: &http.Response{StatusCode: 200},
		mockError:    nil,
	}
	mockActionsService := &mockActionsService{
		mockResponse: &github.Response{Response: &http.Response{StatusCode: 200}},
		mockError:    nil,
	}
	client := githubclient.NewClient(
		mockHTTPClient,
		&multiRepoPRService{services: services},
		&multiRepoIssuesService{services: issueServices},
		mockActionsService,
	)
	prs, err := client.FindOpenPRs(
		context.Background(),
		repos,
		func(models.Repository) config.Filters { return config.Filters{} },
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prs) != repoCount {
		t.Fatalf("expected %d PRs, got %d", repoCount, len(prs))
	}
}

// selectivePRService allows per-PR errors to test best-effort reviewer info enrichment.
type selectivePRService struct {
	mockPRs            []*github.PullRequest
	reviewsByPRNumber  map[int][]*github.PullRequestReview
	commentsByPRNumber map[int][]*github.PullRequestComment
	errByPRNumber      map[int]error
	response           *github.Response
	reviewsResponse    *github.Response
}

func (s *selectivePRService) Get(
	ctx context.Context, owner string, repo string, number int,
) (*github.PullRequest, *github.Response, error) {
	if err, ok := s.errByPRNumber[number]; ok {
		return nil, s.response, err
	}
	for _, pr := range s.mockPRs {
		if pr.GetNumber() == number {
			return pr, s.response, nil
		}
	}
	return nil, s.response, fmt.Errorf("PR not found")
}

func (s *selectivePRService) List(
	ctx context.Context, owner string, repo string, opts *github.PullRequestListOptions,
) ([]*github.PullRequest, *github.Response, error) {
	return s.mockPRs, s.response, nil
}

func (s *selectivePRService) ListReviews(
	ctx context.Context, owner string, repo string, number int, opts *github.ListOptions,
) ([]*github.PullRequestReview, *github.Response, error) {
	reviews := s.reviewsByPRNumber[number]
	err := s.errByPRNumber[number]
	return reviews, s.reviewsResponse, err
}

func (s *selectivePRService) ListComments(
	ctx context.Context, owner string, repo string, number int, opts *github.PullRequestListCommentsOptions,
) ([]*github.PullRequestComment, *github.Response, error) {
	comments := s.commentsByPRNumber[number]
	err := s.errByPRNumber[number]
	return comments, s.reviewsResponse, err
}

// selectiveIssuesService allows per-PR errors to test best-effort issue comment info enrichment.
type selectiveIssuesService struct {
	timelineCommentsByPRNumber map[int][]*github.IssueComment
	errByPRNumber              map[int]error
	response                   *github.Response
}

func (s *selectiveIssuesService) ListComments(
	ctx context.Context, owner string, repo string, number int, opts *github.IssueListCommentsOptions,
) ([]*github.IssueComment, *github.Response, error) {
	comments := s.timelineCommentsByPRNumber[number]
	err := s.errByPRNumber[number]
	return comments, s.response, err
}

func TestFindOpenPRs_ReviewsPartialErrors(t *testing.T) {
	// Two PRs: first reviews fetch fails, second succeeds.
	prService := &selectivePRService{
		mockPRs: []*github.PullRequest{
			{Number: github.Ptr(101), Title: github.Ptr("PR One"), Draft: github.Ptr(false), HTMLURL: github.Ptr("https://example.com/repo/101"), User: &github.User{Login: github.Ptr("author1")}},
			{Number: github.Ptr(102), Title: github.Ptr("PR Two"), Draft: github.Ptr(false), HTMLURL: github.Ptr("https://example.com/repo/102"), User: &github.User{Login: github.Ptr("author2")}},
		},
		reviewsByPRNumber: map[int][]*github.PullRequestReview{
			102: { // success case only
				NewReview("approver2", "Approver Two", "APPROVED"),
				NewReview("commenter2", "Commenter Two", "COMMENTED"),
			},
		},
		commentsByPRNumber: map[int][]*github.PullRequestComment{},
		errByPRNumber: map[int]error{
			101: fmt.Errorf("network timeout"), // failure for first PR
		},
		response:        &github.Response{Response: &http.Response{StatusCode: 200}},
		reviewsResponse: &github.Response{Response: &http.Response{StatusCode: 200}},
	}

	issueService := &selectiveIssuesService{
		timelineCommentsByPRNumber: map[int][]*github.IssueComment{},
		errByPRNumber:              map[int]error{},
		response:                   &github.Response{Response: &http.Response{StatusCode: 200}},
	}

	mockHTTPClient := &mockHTTPClient{
		mockResponse: &http.Response{StatusCode: 200},
		mockError:    nil,
	}
	mockActionsService := &mockActionsService{
		mockResponse: &github.Response{Response: &http.Response{StatusCode: 200}},
		mockError:    nil,
	}
	client := githubclient.NewClient(mockHTTPClient, prService, issueService, mockActionsService)
	repos := []models.Repository{{Owner: "o", Name: "repo"}}
	prs, err := client.FindOpenPRs(
		context.Background(),
		repos,
		func(models.Repository) config.Filters { return config.Filters{} },
	)
	if err != nil {
		t.Fatalf("did not expect error, got %v", err)
	}
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
