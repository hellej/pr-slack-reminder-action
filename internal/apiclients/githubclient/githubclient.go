// Package githubclient provides GitHub API integration for fetching PR data.
// It fetches PR and review data in concurrent batches, and applies
// repository-specific and global filters.
package githubclient

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"slices"

	"time"

	"github.com/google/go-github/v78/github"
	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

type Client interface {
	FindOpenPRs(
		ctx context.Context,
		repositories []models.Repository,
		getFiltersForRepository func(repo models.Repository) config.Filters,
		fetchOptions PRFetchOptions,
	) (OpenPRsResult, error)
	GetPRs(
		ctx context.Context,
		references []models.PullRequestRef,
		getFiltersForRepository func(repo models.Repository) config.Filters,
	) ([]PR, error)
	FetchLatestArtifactByName(
		ctx context.Context,
		owner, repo, artifactName, jsonFilePath string,
		target any,
	) error
}

type GithubActionsService interface {
	ListArtifacts(
		context.Context, string, string, *github.ListArtifactsOptions,
	) (
		*github.ArtifactList, *github.Response, error,
	)
	DownloadArtifact(
		ctx context.Context, owner, repo string, artifactID int64, maxRedirects int,
	) (
		*url.URL, *github.Response, error,
	)
}

type HTTPClient interface {
	Get(url string) (resp *http.Response, err error)
}

type httpClient struct{}

func (h httpClient) Get(url string) (*http.Response, error) {
	return http.Get(url)
}

func NewClient(
	httpClient HTTPClient,
	actionsService GithubActionsService,
	transport graphqlTransport,
) Client {
	return &client{
		http:           httpClient,
		actionsService: actionsService,
		graphql:        graphqlClient{transport: transport},
	}
}

// if the optional tokenForState arg is provided, that will be used for ListArtifacts & DownloadArtifact
// (the main token may not have "actions: read" permission to the current repository, while that is
// necessary for the "update" run-mode where the action first needs to fetch the "state" of the previous
// run)
func GetAuthenticatedClient(token, tokenForState string) Client {
	ghClient := github.NewClient(nil).WithAuthToken(token)

	ghClientForState := ghClient
	if tokenForState != "" {
		ghClientForState = github.NewClient(nil).WithAuthToken(tokenForState)
	}

	return NewClient(
		httpClient{},
		ghClientForState.Actions,
		newHTTPGraphQLTransport(token),
	)
}

type client struct {
	http           HTTPClient
	actionsService GithubActionsService
	graphql        graphqlClient
}

// defaultGitHubAPIConcurrencyLimit caps how many PR batches run at a time,
// to avoid creating excessive simultaneous GitHub API calls.
const defaultGitHubAPIConcurrencyLimit = 3

const MaxPRsToFetch = 50

// Drafts are capped lower than open PRs: only the most recently updated ones are worth showing.
const MaxDraftPRsToFetch = 15

type PRFetchOptions struct {
	IncludeDrafts bool
}

type OpenPRsResult struct {
	PRs []PR
	// Each flag reports that its own bucket was trimmed to its cap. No downstream PR count can
	// tell, since snooze exclusion and content-level pruning shrink the lists further.
	OpenPRsCapped  bool
	DraftPRsCapped bool
}

// Per-call timeout defaults.
const pullRequestListTimeout = 30 * time.Second
const reviewsFetchTimeout = 10 * time.Second

// Returns an error if listing the PRs of any repository fails.
func (c *client) FindOpenPRs(
	ctx context.Context,
	repositories []models.Repository,
	getFiltersForRepository func(repo models.Repository) config.Filters,
	fetchOptions PRFetchOptions,
) (OpenPRsResult, error) {
	log.Printf("Fetching open pull requests for repositories: %v", repositories)

	listedPRs, err := c.listOpenPRs(ctx, repositories)
	if err != nil {
		return OpenPRsResult{}, err
	}

	prResults := utilities.Filter(
		listedPRs,
		getPRFilterFunc[PRResult](getFiltersForRepository, fetchOptions.IncludeDrafts),
	)
	prResults, openPRsCapped, draftPRsCapped := capOpenPRResultsToLimit(
		prResults, fetchOptions.IncludeDrafts,
	)
	logFoundPRs(prResults, fetchOptions.IncludeDrafts)

	prs, err := c.enrichPRs(ctx, prResults)
	if err != nil {
		return OpenPRsResult{}, err
	}
	return OpenPRsResult{
		PRs:            excludeSnoozedPRs(prs),
		OpenPRsCapped:  openPRsCapped,
		DraftPRsCapped: draftPRsCapped,
	}, nil
}

func (c *client) GetPRs(
	ctx context.Context,
	references []models.PullRequestRef,
	getFiltersForRepository func(repo models.Repository) config.Filters,
) ([]PR, error) {
	if len(references) > MaxPRsToFetch {
		log.Printf(
			"More than %d PRs requested (%d), fetching only the first %d",
			MaxPRsToFetch, len(references), MaxPRsToFetch,
		)
		references = references[:MaxPRsToFetch]
	} else {
		log.Printf("Fetching %d pull requests", len(references))
	}

	fetchedPRs, err := c.getPRsByRef(ctx, references)
	if err != nil {
		return nil, err
	}

	prs := utilities.Filter(fetchedPRs, getPRFilterFunc[PR](getFiltersForRepository, false))
	prs = capPRsToLimit(prs)
	logFoundPRs(prs, false)

	return excludeSnoozedPRs(prs), nil
}

func getPRFilterFunc[T repositoryPullRequest](
	getFiltersForRepository func(repo models.Repository) config.Filters,
	includeDrafts bool,
) func(item T) bool {
	return func(item T) bool {
		pullRequest := item.getPullRequest()
		return (includeDrafts || !pullRequest.GetDraft()) &&
			includePR(pullRequest, getFiltersForRepository(item.getRepository()))
	}
}

func logFoundPRs[T repositoryPullRequest](prs []T, includeDrafts bool) {
	if includeDrafts {
		drafts := utilities.Filter(prs, isDraftPR)
		log.Printf("Found %d open pull requests, %d of them drafts:", len(prs), len(drafts))
	} else {
		log.Printf("Found %d open pull requests:", len(prs))
	}
	for _, item := range prs {
		log.Printf("%s/%v", item.getRepository().GetPath(), item.getPullRequest().GetNumber())
	}
}

func isDraftPR[T repositoryPullRequest](item T) bool {
	return item.getPullRequest().GetDraft()
}

// Caps drafts and non-drafts separately, so an over-cap draft fetch can never displace open PRs
// the reminder message would otherwise show.
func capOpenPRResultsToLimit(
	prResults []PRResult, includeDrafts bool,
) (capped []PRResult, openPRsCapped, draftPRsCapped bool) {
	if !includeDrafts {
		capped = capPRsToLimit(prResults)
		return capped, len(capped) < len(prResults), false
	}

	openPRs := utilities.Filter(prResults, func(item PRResult) bool { return !isDraftPR(item) })
	drafts := utilities.Filter(prResults, isDraftPR)

	cappedOpenPRs := capPRsToLimit(openPRs)
	cappedDrafts := capDraftPRResultsToLimit(drafts)

	return slices.Concat(cappedOpenPRs, cappedDrafts),
		len(cappedOpenPRs) < len(openPRs),
		len(cappedDrafts) < len(drafts)
}

// Keeps the most recently updated drafts: the canvas orders and prunes its WIP list by activity,
// and update time is the closest proxy available before enrichment.
func capDraftPRResultsToLimit(drafts []PRResult) []PRResult {
	if len(drafts) <= MaxDraftPRsToFetch {
		return drafts
	}
	log.Printf(
		"More than %d draft pull requests found (%d), including only the %d most recently updated",
		MaxDraftPRsToFetch, len(drafts), MaxDraftPRsToFetch,
	)
	slices.SortStableFunc(drafts, func(a, b PRResult) int {
		return b.getPullRequest().GetUpdatedAt().Compare(a.getPullRequest().GetUpdatedAt())
	})
	return drafts[:MaxDraftPRsToFetch]
}

func capPRsToLimit[T repositoryPullRequest](prs []T) []T {
	if len(prs) <= MaxPRsToFetch {
		return prs
	}
	log.Printf(
		"More than %d pull requests found (%d), including only the latest %d",
		MaxPRsToFetch, len(prs), MaxPRsToFetch,
	)
	slices.SortStableFunc(prs, func(a, b T) int {
		createdA, createdB := a.getPullRequest().GetCreatedAt(), b.getPullRequest().GetCreatedAt()
		if !createdA.Equal(createdB) {
			return createdB.Compare(createdA)
		}
		return b.getPullRequest().GetUpdatedAt().Compare(a.getPullRequest().GetUpdatedAt())
	})
	return prs[:MaxPRsToFetch]
}
