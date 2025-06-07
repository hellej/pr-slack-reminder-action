package githubclient

import (
	"context"
	"log"
	"slices"
	"strings"

	"github.com/google/go-github/v72/github"
)

func GetClient(token string) *github.Client {
	return github.NewClient(nil).WithAuthToken(token)
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

func sortPRsByCreatedAt(prs []*github.PullRequest) []*github.PullRequest {
	slices.SortFunc(prs, func(a, b *github.PullRequest) int {
		if a.GetCreatedAt().After(b.GetCreatedAt().Time) {
			return -1
		}
		if a.GetCreatedAt().Before(b.GetCreatedAt().Time) {
			return 1
		}
		if a.GetUpdatedAt().After(b.GetUpdatedAt().Time) {
			return -1
		}
		if a.GetUpdatedAt().Before(b.GetUpdatedAt().Time) {
			return 1
		}
		return 0
	})
	return prs
}

func logFoundPRs(prs []*github.PullRequest) []*github.PullRequest {
	if len(prs) == 0 {
		log.Println("No open pull requests found")
	} else {
		log.Printf("Found %d open pull requests:", len(prs))
	}
	for _, pr := range prs {
		log.Printf("#%v: %s \"%s\"", *pr.Number, pr.GetHTMLURL(), pr.GetTitle())
	}
	return prs
}

func FetchOpenPRs(client *github.Client, repository string) []*github.PullRequest {
	repoOwner, repoName := parseOwnerAndRepo(repository)
	log.Printf("Fetching PRs from repository: %s/%s", repoOwner, repoName)
	prs, response, err := client.PullRequests.List(context.Background(), repoOwner, repoName, nil)
	if err != nil {
		if response != nil && response.StatusCode == 404 {
			log.Fatalf("Repository %s/%s not found. Check the repository name and permissions.", repoOwner, repoName)
		} else {
			log.Fatalf("Error fetching pull requests: %v", err)
		}
	}
	return logFoundPRs(sortPRsByCreatedAt(prs))
}
