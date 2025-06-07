package githubclient

import (
	"context"
	"log"
	"strings"

	"github.com/google/go-github/v72/github"
)

type Client interface {
	FetchOpenPRs(repository string) []*github.PullRequest
}

type githubPullRequestsService interface {
	List(ctx context.Context, owner string, repo string, opts *github.PullRequestListOptions) ([]*github.PullRequest, *github.Response, error)
}

type client struct {
	prsService githubPullRequestsService
}

func NewClient(prsService githubPullRequestsService) Client {
	return &client{prsService: prsService}
}

func GetAuthenticatedClient(token string) Client {
	ghClient := github.NewClient(nil).WithAuthToken(token)
	return NewClient(ghClient.PullRequests)
}

func (c *client) FetchOpenPRs(repository string) []*github.PullRequest {
	repoOwner, repoName := parseOwnerAndRepo(repository)
	log.Printf("Fetching PRs from repository: %s/%s", repoOwner, repoName)
	prs, response, err := c.prsService.List(context.Background(), repoOwner, repoName, nil)
	if err != nil {
		if response != nil && response.StatusCode == 404 {
			log.Panicf("Repository %s/%s not found. Check the repository name and permissions.", repoOwner, repoName)
		} else {
			log.Panicf("Error fetching pull requests: %v", err)
		}
	}
	return prs
}

func parseOwnerAndRepo(repository string) (string, string) {
	repoParts := strings.Split(repository, "/")
	if len(repoParts) != 2 {
		log.Panicf("Invalid GITHUB_REPOSITORY format: %s", repository)
	}
	repoOwner := repoParts[0]
	repoName := repoParts[1]

	return repoOwner, repoName
}
