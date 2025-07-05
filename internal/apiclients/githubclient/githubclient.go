package githubclient

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/google/go-github/v72/github"
	"github.com/hellej/pr-slack-reminder-action/internal/config"
)

type Client interface {
	FetchOpenPRs(
		repositories []config.Repository,
		globalFilters config.Filters,
		repositoryFilters map[string]config.Filters,
	) ([]PR, error)
}

type githubPullRequestsService interface {
	List(
		ctx context.Context, owner string, repo string, opts *github.PullRequestListOptions,
	) (
		[]*github.PullRequest, *github.Response, error,
	)
	ListReviews(
		ctx context.Context, owner string, repo string, number int, opts *github.ListOptions,
	) (
		[]*github.PullRequestReview, *github.Response, error,
	)
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
func (c *client) FetchOpenPRs(
	repositories []config.Repository,
	globalFilters config.Filters,
	repositoryFilters map[string]config.Filters,
) ([]PR, error) {
	log.Printf("Fetching open pull requests for repositories: %v", repositories)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	apiResultChannel := make(chan PRsOfRepoResult, len(repositories))

	for _, repo := range repositories {
		wg.Add(1)
		go func(r config.Repository) {
			defer wg.Done()
			apiResult := c.fetchOpenPRsForRepository(ctx, r.Owner, r.Name)
			apiResultChannel <- apiResult
			if apiResult.err != nil {
				cancel()
			} else {
				logFoundPRs(r.Path, apiResult.prs)
			}
		}(repo)
	}

	go func() {
		wg.Wait()
		close(apiResultChannel)
	}()

	successfulResults := []PRsOfRepoResult{}
	for result := range apiResultChannel {
		if result.err != nil {
			return nil, result.err
		} else {
			successfulResults = append(successfulResults, result)
		}
	}

	return filterPRs(
		c.addReviewerInfoToPRs(successfulResults),
		globalFilters,
		repositoryFilters,
	), nil
}

func (c *client) fetchOpenPRsForRepository(
	ctx context.Context, repoOwner string, repoName string,
) PRsOfRepoResult {
	prs, response, err := c.prsService.List(ctx, repoOwner, repoName, nil)
	if err != nil {
		if response != nil && response.StatusCode == 404 {
			return PRsOfRepoResult{
				prs:        nil,
				owner:      repoOwner,
				repository: repoName,
				err: fmt.Errorf(
					"repository %s/%s not found - check the repository name and permissions",
					repoOwner,
					repoName,
				)}

		} else {
			return PRsOfRepoResult{
				prs: nil,
				err: fmt.Errorf("error fetching pull requests from %s/%s: %v", repoOwner, repoName, err),
			}
		}
	}
	return PRsOfRepoResult{
		prs:        prs,
		owner:      repoOwner,
		repository: repoName,
		err:        nil,
	}
}

func logFoundPRs(repository string, prs []*github.PullRequest) {
	log.Printf("Found %d open pull requests in repository %s:", len(prs), repository)
	for _, pr := range prs {
		log.Printf("  #%v: %s \"%s\"", *pr.Number, pr.GetHTMLURL(), pr.GetTitle())
	}
}

func (c *client) addReviewerInfoToPRs(prResults []PRsOfRepoResult) []PR {
	log.Printf("Fetching pull request reviewers for PRs")

	totalPRCount := 0
	for _, result := range prResults {
		totalPRCount += result.GetPRCount()
	}

	resultChannel := make(chan FetchReviewsResult, totalPRCount)
	var wg sync.WaitGroup

	for _, result := range prResults {
		for _, pullRequest := range result.prs {
			wg.Add(1)
			go func(owner string, repo string, pr *github.PullRequest) {
				defer wg.Done()
				reviews, response, err := c.prsService.ListReviews(context.Background(), owner, repo, *pr.Number, nil)
				if err != nil {
					err = fmt.Errorf(
						"error fetching reviews for pull request %s/%s#%d: %v/%v",
						owner,
						repo,
						*pr.Number,
						response.Status,
						err,
					)
				}
				prWithReviews := FetchReviewsResult{
					pr:         pr,
					reviews:    reviews,
					repository: repo,
					err:        err,
				}
				resultChannel <- prWithReviews

			}(result.owner, result.repository, pullRequest)
		}
	}

	go func() {
		wg.Wait()
		close(resultChannel)
	}()

	allPRs := []PR{}
	for result := range resultChannel {
		result.printResult()
		allPRs = append(allPRs, result.asPR())
	}
	return allPRs
}
