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

type client struct {
	client *github.Client
}

func GetClient(token string) Client {
	return client{client: github.NewClient(nil).WithAuthToken(token)}
}

func (c client) FetchOpenPRs(repository string) []*github.PullRequest {
	repoOwner, repoName := parseOwnerAndRepo(repository)
	log.Printf("Fetching PRs from repository: %s/%s", repoOwner, repoName)
	prs, response, err := c.client.PullRequests.List(context.Background(), repoOwner, repoName, nil)
	if err != nil {
		if response != nil && response.StatusCode == 404 {
			log.Fatalf("Repository %s/%s not found. Check the repository name and permissions.", repoOwner, repoName)
		} else {
			log.Fatalf("Error fetching pull requests: %v", err)
		}
	}
	return prs
}

func parseOwnerAndRepo(repository string) (string, string) {
	repoParts := strings.Split(repository, "/")
	if len(repoParts) != 2 {
		log.Fatalf("Invalid GITHUB_REPOSITORY format: %s", repository)
	}
	repoOwner := repoParts[0]
	repoName := repoParts[1]

	return repoOwner, repoName
}
