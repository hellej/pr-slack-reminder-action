package githubclient

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"

	"github.com/google/go-github/v72/github"
)

type Client interface {
	FetchOpenPRs(repositories []string) ([]PR, error)
}

type githubPullRequestsService interface {
	List(ctx context.Context, owner string, repo string, opts *github.PullRequestListOptions) ([]*github.PullRequest, *github.Response, error)
	ListReviews(ctx context.Context, owner string, repo string, number int, opts *github.ListOptions) ([]*github.PullRequestReview, *github.Response, error)
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

type PR struct {
	*github.PullRequest
	Repository       string
	CommentedByUsers []string
	ApprovedByUsers  []string
}

// Returns an error if fetching PRs from any repository fails (and cancels other requests).
//
// The wait group & cancellation logic could be refactored to use errgroup package for more
// concise implementation. However, the current implementation also serves as learning material
// so we can save the refactoring for later...
func (c *client) FetchOpenPRs(repositoryPaths []string) ([]PR, error) {
	log.Printf("Fetching open pull requests for repositories: %v", repositoryPaths)

	parsedRepos, err := parseRepositoryNames(repositoryPaths)
	if err != nil {
		return nil, fmt.Errorf("unable to parse repository input: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	apiResultChannel := make(chan PRsOfRepoResult, len(repositoryPaths))

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

	successfulResults := []PRsOfRepoResult{}
	for result := range apiResultChannel {
		if result.err != nil {
			return nil, result.err
		} else {
			successfulResults = append(successfulResults, result)
		}
	}

	return c.AddReviewerInfoToPRs(successfulResults), nil
}

type PRsOfRepoResult struct {
	prs        []*github.PullRequest
	repository string
	owner      string
	err        error
}

func (r PRsOfRepoResult) GetPRCount() int {
	if r.prs != nil {
		return len(r.prs)
	}
	return 0
}

func (c *client) fetchOpenPRsForRepository(ctx context.Context, repoOwner string, repoName string) PRsOfRepoResult {
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

func (c *client) AddReviewerInfoToPRs(prResults []PRsOfRepoResult) []PR {
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
		result.PrintResult()
		allPRs = append(allPRs, result.AsPR())
	}
	return allPRs
}

type FetchReviewsResult struct {
	pr         *github.PullRequest
	reviews    []*github.PullRequestReview
	repository string
	err        error
}

func (r FetchReviewsResult) PrintResult() {
	if r.err != nil {
		log.Printf("Unable to fetch reviews for PR #%d: %v", r.pr.GetNumber(), r.err)
	} else {
		log.Printf("Found %d reviews for PR %v/%d", len(r.reviews), r.repository, r.pr.GetNumber())
	}
	for _, review := range r.reviews {
		log.Printf("Review by %s: %s", review.GetUser().GetLogin(), *review.State)
	}
}

func (r FetchReviewsResult) AsPR() PR {
	approvedByUsers := []string{}
	commentedByUsers := []string{}

	for _, review := range r.reviews {
		user := review.GetUser().GetLogin()
		if user == "" {
			continue
		}
		if review.GetState() == "APPROVED" {
			if !slices.Contains(approvedByUsers, user) {
				approvedByUsers = append(approvedByUsers, user)
			}
		} else {
			if !slices.Contains(commentedByUsers, user) && !slices.Contains(approvedByUsers, user) {
				// Only add to commentedByUsers if the user has not already approved
				commentedByUsers = append(commentedByUsers, user)
			}
		}
	}

	return PR{
		PullRequest:      r.pr,
		Repository:       r.repository,
		CommentedByUsers: approvedByUsers,
		ApprovedByUsers:  commentedByUsers,
	}
}
