package mockgithubclient

import (
	"context"
	"net/http"

	"github.com/google/go-github/v72/github"
	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
)

func MakeMockGitHubClientGetter(
	prs []*github.PullRequest,
	prsByRepo map[string][]*github.PullRequest,
	listPRsResponseStatus int,
	listPRsErr error,
	reviewsByPRNumber map[int][]*github.PullRequestReview,
	commentsByPRNumber map[int][]*github.PullRequestComment,
) func(token string) githubclient.Client {
	return func(token string) githubclient.Client {
		mockPRService := &mockPullRequestService{
			mockPRs:                prs,
			mockPRsByRepo:          prsByRepo,
			mockReviewsByPRNumber:  reviewsByPRNumber,
			mockCommentsByPRNumber: commentsByPRNumber,
			mockResponse: &github.Response{
				Response: &http.Response{
					StatusCode: listPRsResponseStatus,
				},
			},
			mockError: listPRsErr,
		}
		mockIssueService := &mockIssueService{
			mockTimelineCommentsByPRNumber: map[int][]*github.IssueComment{},
			mockResponse: &github.Response{
				Response: &http.Response{
					StatusCode: 200,
				},
			},
			mockError: nil,
		}
		return githubclient.NewClient(mockPRService, mockIssueService)
	}
}

func NewReview(id int64, state, login, name, body string, userType ...string) *github.PullRequestReview {
	var t *string
	if len(userType) > 0 && userType[0] != "" {
		t = github.Ptr(userType[0])
	}
	var b *string
	if body != "" {
		b = github.Ptr(body)
	}
	return &github.PullRequestReview{
		ID:   github.Ptr(id),
		Body: b,
		User: &github.User{
			Login: github.Ptr(login),
			Name:  github.Ptr(name),
			Type:  t,
		},
		State: github.Ptr(state),
	}
}

func NewComment(id int64, login, name, body string, userType ...string) *github.PullRequestComment {
	var t *string
	if len(userType) > 0 && userType[0] != "" {
		t = github.Ptr(userType[0])
	}
	var b *string
	if body != "" {
		b = github.Ptr(body)
	}
	return &github.PullRequestComment{
		ID:   github.Ptr(id),
		Body: b,
		User: &github.User{
			Login: github.Ptr(login),
			Name:  github.Ptr(name),
			Type:  t,
		},
	}
}

type mockPullRequestService struct {
	mockPRs                []*github.PullRequest
	mockPRsByRepo          map[string][]*github.PullRequest
	mockReviewsByPRNumber  map[int][]*github.PullRequestReview
	mockCommentsByPRNumber map[int][]*github.PullRequestComment
	mockResponse           *github.Response
	mockError              error
}

func (m *mockPullRequestService) List(
	ctx context.Context, owner string, repo string, opts *github.PullRequestListOptions,
) ([]*github.PullRequest, *github.Response, error) {
	if m.mockPRsByRepo != nil {
		return m.mockPRsByRepo[repo], m.mockResponse, m.mockError
	}
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
