package githubclient

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

// search returns issues and PRs, so the PR fields are selected through an inline fragment and
// need no named one. The selection is narrower than the open-PR listing's: a merged row shows no
// draft marker, no activity and no head commit.
const mergedPullRequestSearchSelection = `{ issueCount nodes { ... on PullRequest {
    number title url createdAt mergedAt
    author { login __typename ... on User { name } }
    labels(first: 100){ nodes { name } }
  } } }`

// GitHub's maximum page size for a search connection, and the point past which a repository's
// merges no longer fit in one page.
const mergedPRSearchPageSize = 100

const mergedSinceDayLayout = "2006-01-02"

// One aliased search per repository, so each result set maps back to its repository without
// selecting the repository on every node.
func buildMergedPRsSearchQuery(
	repositories []models.Repository, mergedSince time.Time,
) graphqlQuery {
	query := graphqlQuery{
		variables:         map[string]any{},
		aliases:           make([]string, len(repositories)),
		repositoryByAlias: map[string]models.Repository{},
	}
	variableDeclarations := make([]string, len(repositories))
	searchSelections := make([]string, len(repositories))

	for index, repository := range repositories {
		alias := fmt.Sprintf("s%d", index)
		searchQueryVariable := fmt.Sprintf("q%d", index)

		query.aliases[index] = alias
		query.repositoryByAlias[alias] = repository
		query.variables[searchQueryVariable] = mergedPRSearchQueryString(repository, mergedSince)
		variableDeclarations[index] = "$" + searchQueryVariable + ":String!"
		searchSelections[index] = fmt.Sprintf(
			"  %s: search(query:$%s, type: ISSUE, first: %d)%s",
			alias, searchQueryVariable, mergedPRSearchPageSize, mergedPullRequestSearchSelection,
		)
	}

	query.text = assembleQuery(variableDeclarations, searchSelections, "")
	return query
}

// The cutoff is a day rather than a timestamp, so the search returns up to one extra day of
// merges; the exact cut happens client-side.
func mergedPRSearchQueryString(repository models.Repository, mergedSince time.Time) string {
	return fmt.Sprintf(
		"repo:%s is:pr is:merged merged:>=%s",
		repository.GetPath(), mergedSince.UTC().Format(mergedSinceDayLayout),
	)
}

type searchResultsNode struct {
	IssueCount int               `json:"issueCount"`
	Nodes      []pullRequestNode `json:"nodes"`
}

// Any error fails the whole merged fetch: search reports a nonexistent repository, one the token
// cannot read and an empty window identically, so there is no per-repository failure to report.
func (c *client) searchMergedPRs(
	ctx context.Context, repositories []models.Repository, mergedSince time.Time,
) ([]PRResult, error) {
	query := buildMergedPRsSearchQuery(repositories, mergedSince)

	callCtx, cancel := context.WithTimeout(ctx, pullRequestListTimeout)
	defer cancel()

	var data aliasedData[searchResultsNode]
	fieldErrors, err := c.graphql.Do(callCtx, query.text, query.variables, query.aliases, &data)
	if err != nil {
		return nil, mergedPRsFetchError(err)
	}
	if len(fieldErrors) > 0 {
		return nil, mergedPRsFetchError(fieldErrors[0])
	}
	logRateLimit(data.RateLimit)

	prResults := []PRResult{}
	for _, alias := range query.aliases {
		repository := query.repositoryByAlias[alias]
		node := data.ByAlias[alias]
		if node == nil {
			return nil, mergedPRsFetchError(
				fmt.Errorf("no data returned for %s", repository.GetPath()),
			)
		}
		logTruncatedMergedPRs(repository, node.IssueCount)
		prResults = append(
			prResults, utilities.Map(node.Nodes, mergedPRResultMapper(repository))...,
		)
	}
	return prResults, nil
}

func mergedPRsFetchError(err error) error {
	return fmt.Errorf("error fetching merged pull requests: %w", err)
}

// The list is not paged, so a repository merging more than a page of PRs inside the window is
// missing some of them, visible nowhere but the log.
func logTruncatedMergedPRs(repository models.Repository, issueCount int) {
	if issueCount <= mergedPRSearchPageSize {
		return
	}
	log.Printf(
		"%s merged %d pull requests within the window, listing only the newest %d of them",
		repository.GetPath(), issueCount, mergedPRSearchPageSize,
	)
}

func mergedPRResultMapper(repository models.Repository) func(node pullRequestNode) PRResult {
	return func(node pullRequestNode) PRResult {
		return PRResult{pr: mergedPullRequestFromNode(node), repository: repository}
	}
}

// is:merged guarantees the state and the merged flag, which are therefore not selected.
func mergedPullRequestFromNode(node pullRequestNode) *PullRequest {
	pullRequest := pullRequestFromNode(node)
	pullRequest.State = closedPullRequestState
	pullRequest.Merged = true
	return pullRequest
}
