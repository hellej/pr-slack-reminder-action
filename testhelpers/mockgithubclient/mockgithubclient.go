package mockgithubclient

import (
	"context"
	"net/http"

	"github.com/google/go-github/v72/github"
	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
)

func MakeMockGitHubClientGetter(
	prs []*github.PullRequest,
	responseStatus int,
	err error,
) func(token string) githubclient.Client {
	return func(token string) githubclient.Client {
		return githubclient.NewClient(&mockPullRequestsService{
			mockPRs: prs,
			mockResponse: &github.Response{
				Response: &http.Response{
					StatusCode: responseStatus,
				},
			},
			mockError: err,
		})
	}
}

func (m *mockPullRequestsService) List(
	ctx context.Context, owner string, repo string, opts *github.PullRequestListOptions,
) ([]*github.PullRequest, *github.Response, error) {
	return m.mockPRs, m.mockResponse, m.mockError
}

type mockPullRequestsService struct {
	mockPRs      []*github.PullRequest
	mockResponse *github.Response
	mockError    error
}
