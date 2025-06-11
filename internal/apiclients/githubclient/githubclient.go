package githubclient

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/google/go-github/v72/github"
)

type Client interface {
	FetchOpenPRs(repositories []string) ([]*github.PullRequest, error)
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

func (c *client) FetchOpenPRs(repositories []string) ([]*github.PullRequest, error) {
	allPRs := []*github.PullRequest{}
	for _, repository := range repositories {
		repoOwner, repoName, err := parseOwnerAndRepo(repository)
		if err != nil {
			return nil, fmt.Errorf("error parsing repository name %s: %v", repository, err)
		}
		log.Printf("Fetching PRs from repository: %s/%s", repoOwner, repoName)
		prs, err := c.fetchOpenPRsForRepository(repoOwner, repoName)
		if err != nil {
			return nil, err
		}
		logFoundPRs(prs)
		allPRs = append(allPRs, prs...)
	}
	return allPRs, nil
}

func (c *client) fetchOpenPRsForRepository(repoOwner string, repoName string) ([]*github.PullRequest, error) {
	prs, response, err := c.prsService.List(context.Background(), repoOwner, repoName, nil)
	if err != nil {
		if response != nil && response.StatusCode == 404 {
			return nil, fmt.Errorf(
				"repository %s/%s not found - check the repository name and permissions",
				repoOwner,
				repoName,
			)
		} else {
			return nil, fmt.Errorf("error fetching pull requests from %s/%s: %v", repoOwner, repoName, err)
		}
	}
	return prs, nil
}

func parseOwnerAndRepo(repository string) (string, string, error) {
	repoParts := strings.Split(repository, "/")
	if len(repoParts) != 2 {
		return "", "", fmt.Errorf("invalid GITHUB_REPOSITORY format: %s", repository)
	}
	repoOwner := repoParts[0]
	repoName := repoParts[1]

	return repoOwner, repoName, nil
}

func logFoundPRs(prs []*github.PullRequest) {
	if len(prs) == 0 {
		log.Println("No open pull requests found")
	} else {
		log.Printf("Found %d open pull requests:", len(prs))
	}
	for _, pr := range prs {
		log.Printf("#%v: %s \"%s\"", *pr.Number, pr.GetHTMLURL(), pr.GetTitle())
	}
}
