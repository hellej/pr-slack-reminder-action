package githubclient

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

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

// Returns an error if fetching PRs from any repository fails (and cancels other requests).
//
// The wait group & cancellation logic could be refactored to use errgroup package for more
// concise implementation. However, the current implementation also serves as learning material
// so we can save the refactoring for later...
func (c *client) FetchOpenPRs(repositories []string) ([]*github.PullRequest, error) {
	log.Printf("Fetching open pull requests for repositories: %v", repositories)

	parsedRepos, err := parseRepositoryNames(repositories)
	if err != nil {
		return nil, fmt.Errorf("unable to parse repository input: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	apiResultChannel := make(chan FetchPRsResult, len(repositories))
	allPRs := []*github.PullRequest{}

	for _, ownerAndRepo := range parsedRepos {
		wg.Add(1)
		go func(r OwnerAndRepo) {
			defer wg.Done()
			apiResult := c.fetchOpenPRsForRepository(ctx, r.Owner, r.Repo)
			apiResultChannel <- apiResult
			if apiResult.err != nil {
				cancel()
			} else {
				logFoundPRs(r.Repo, apiResult.prs)
			}
		}(ownerAndRepo)
	}

	go func() {
		wg.Wait()
		close(apiResultChannel)
	}()

	for results := range apiResultChannel {
		if results.err != nil {
			return nil, results.err
		} else {
			allPRs = append(allPRs, results.prs...)
		}
	}

	return allPRs, nil
}

type FetchPRsResult struct {
	prs []*github.PullRequest
	err error
}

func (c *client) fetchOpenPRsForRepository(ctx context.Context, repoOwner string, repoName string) FetchPRsResult {
	prs, response, err := c.prsService.List(ctx, repoOwner, repoName, nil)
	if err != nil {
		if response != nil && response.StatusCode == 404 {
			return FetchPRsResult{
				prs: nil,
				err: fmt.Errorf(
					"repository %s/%s not found - check the repository name and permissions",
					repoOwner,
					repoName,
				)}

		} else {
			return FetchPRsResult{
				prs: nil,
				err: fmt.Errorf("error fetching pull requests from %s/%s: %v", repoOwner, repoName, err),
			}
		}
	}
	return FetchPRsResult{prs: prs, err: nil}
}

func parseRepositoryNames(repositories []string) ([]OwnerAndRepo, error) {
	results := make([]OwnerAndRepo, len(repositories))
	for i, repository := range repositories {
		repoOwner, repoName, err := parseOwnerAndRepo(repository)
		if err != nil {
			return nil, err
		}
		results[i] = OwnerAndRepo{Owner: repoOwner, Repo: repoName}
	}
	return results, nil
}

type OwnerAndRepo struct {
	Owner string
	Repo  string
}

func parseOwnerAndRepo(repository string) (string, string, error) {
	repoParts := strings.Split(repository, "/")
	if len(repoParts) != 2 {
		return "", "", fmt.Errorf("invalid owner/repository format: %s", repository)
	}
	repoOwner := repoParts[0]
	repoName := repoParts[1]

	return repoOwner, repoName, nil
}

func logFoundPRs(repository string, prs []*github.PullRequest) {
	log.Printf("Found %d open pull requests in repository %s:", len(prs), repository)
	for _, pr := range prs {
		log.Printf("#%v: %s \"%s\"", *pr.Number, pr.GetHTMLURL(), pr.GetTitle())
	}
}
