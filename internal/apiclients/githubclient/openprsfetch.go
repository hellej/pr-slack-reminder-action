package githubclient

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log"
	"slices"

	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
	"golang.org/x/sync/errgroup"
)

// labels are capped at GitHub's maximum page size; REST returned them unpaginated.
const openPullRequestsFragment = `fragment prs on Repository {
  pullRequests(states: OPEN, first: 100, orderBy: {field: CREATED_AT, direction: DESC}) {
    nodes {
      number title url isDraft createdAt updatedAt headRefOid
      author { login __typename ... on User { name } }
      labels(first: 100) { nodes { name } }
    }
  }
}`

// Aliases cannot be variables, so they are generated into the query text; owner and name are
// always passed as variables.
func buildListOpenPRsQuery(repositories []models.Repository) graphqlQuery {
	query := graphqlQuery{
		variables:         map[string]any{},
		aliases:           make([]string, len(repositories)),
		repositoryByAlias: map[string]models.Repository{},
	}
	variableDeclarations := make([]string, 0, 2*len(repositories))
	repositorySelections := make([]string, len(repositories))

	for index, repository := range repositories {
		alias := fmt.Sprintf("r%d", index)
		ownerVariable := fmt.Sprintf("owner%d", index)
		nameVariable := fmt.Sprintf("name%d", index)

		query.aliases[index] = alias
		query.repositoryByAlias[alias] = repository
		query.variables[ownerVariable] = repository.Owner
		query.variables[nameVariable] = repository.Name
		variableDeclarations = append(
			variableDeclarations, "$"+ownerVariable+":String!", "$"+nameVariable+":String!",
		)
		repositorySelections[index] = fmt.Sprintf(
			"  %s: repository(owner:$%s,name:$%s){ ...prs }", alias, ownerVariable, nameVariable,
		)
	}

	query.text = assembleQuery(variableDeclarations, repositorySelections, openPullRequestsFragment)
	return query
}

type repositoryPullRequestsNode struct {
	PullRequests connection[pullRequestNode] `json:"pullRequests"`
}

func (c *client) listOpenPRs(
	ctx context.Context, repositories []models.Repository,
) ([]PRResult, error) {
	query := buildListOpenPRsQuery(repositories)

	callCtx, cancel := context.WithTimeout(ctx, pullRequestListTimeout)
	defer cancel()

	var data aliasedData[repositoryPullRequestsNode]
	fieldErrors, err := c.graphql.Do(callCtx, query.text, query.variables, query.aliases, &data)
	if err != nil {
		return nil, listOpenPRsError(err, query.repositoryByAlias)
	}
	// A field error here loses every PR of one repository, which has always failed the whole list.
	if len(fieldErrors) > 0 {
		return nil, repositoryFetchError(query.repositoryByAlias[fieldErrors[0].alias], fieldErrors[0])
	}
	logRateLimit(data.RateLimit)

	prResults := []PRResult{}
	for _, alias := range query.aliases {
		repository := query.repositoryByAlias[alias]
		node := data.ByAlias[alias]
		if node == nil {
			return nil, repositoryFetchError(repository, errors.New("no data returned"))
		}
		prResults = append(
			prResults,
			utilities.Map(node.PullRequests.Nodes, openPRResultMapper(repository))...,
		)
	}
	return prResults, nil
}

func listOpenPRsError(err error, repositoryByAlias map[string]models.Repository) error {
	var repoError repositoryError
	if errors.As(err, &repoError) {
		if repository, isKnownAlias := repositoryByAlias[repoError.alias]; isKnownAlias {
			return fmt.Errorf(
				"repository %s/%s not found - check the repository name and permissions",
				repository.Owner,
				repository.Name,
			)
		}
	}
	return fmt.Errorf("error fetching pull requests: %w", err)
}

func repositoryFetchError(repository models.Repository, err error) error {
	return fmt.Errorf(
		"error fetching pull requests from %s/%s: %w", repository.Owner, repository.Name, err,
	)
}

func openPRResultMapper(repository models.Repository) func(node pullRequestNode) PRResult {
	return func(node pullRequestNode) PRResult {
		return PRResult{pr: openPullRequestFromNode(node), repository: repository}
	}
}

// states: OPEN guarantees the state, which is therefore not selected.
func openPullRequestFromNode(node pullRequestNode) *PullRequest {
	pullRequest := pullRequestFromNode(node)
	pullRequest.State = openPullRequestState
	pullRequest.Merged = false
	return pullRequest
}

// commits are selected for the PR tracker canvas, as the last-activity timestamp.
const enrichedPullRequestSelection = `  number
  commits(last: 1){ nodes { commit { oid committedDate } } }
  reviews(first: 100){ nodes { state author { login __typename ... on User { name } } } }
  comments(first: 100){ nodes { createdAt body author { login __typename ... on User { name } } } }`

var enrichedPullRequestFragment = newPullRequestFragment("enrichedPr", enrichedPullRequestSelection)

// Fetches reviews and comments for the given PRs and returns them enriched, in input order.
// Enrichment failures scoped to one PR are logged and leave that PR without reviewer info.
func (c *client) enrichPRs(ctx context.Context, prResults []PRResult) ([]PR, error) {
	log.Printf("\nFetching pull request reviews and comments for PRs")

	batches := slices.Collect(slices.Chunk(prResults, enrichBatchSize))
	enrichedBatches := make([][]PR, len(batches))

	batchGroup, batchCtx := errgroup.WithContext(ctx)
	batchGroup.SetLimit(defaultGitHubAPIConcurrencyLimit)

	for index, batch := range batches {
		batchGroup.Go(func() error {
			enriched, err := c.enrichPRBatch(batchCtx, batch)
			enrichedBatches[index] = enriched
			return err
		})
	}
	if err := batchGroup.Wait(); err != nil {
		return nil, err
	}
	return utilities.FlatMap(enrichedBatches), nil
}

func (c *client) enrichPRBatch(ctx context.Context, batch []PRResult) ([]PR, error) {
	query := buildPullRequestsQuery(
		utilities.Map(batch, pullRequestRefOfResult), enrichedPullRequestFragment,
	)

	callCtx, cancel := context.WithTimeout(ctx, reviewsFetchTimeout)
	defer cancel()

	var data aliasedData[pullRequestWrapperNode]
	fieldErrors, err := c.graphql.Do(callCtx, query.text, query.variables, query.aliases, &data)
	if err != nil && !failsOnlyOnePullRequest(err) {
		return nil, enrichPRsError(err, query.repositoryByAlias)
	}
	logRateLimit(data.RateLimit)

	enriched := make([]PR, len(batch))
	for index, result := range batch {
		alias := query.aliases[index]
		enriched[index] = enrichedPR(result, data.ByAlias[alias], errorsForAlias(alias, err, fieldErrors))
	}
	return enriched, nil
}

// Enrichment has never been able to fail the fetch, so a PR closed between the phases degrades
// to the same per-PR path as a field error instead of aborting the run.
func failsOnlyOnePullRequest(err error) bool {
	var prError pullRequestError
	return errors.As(err, &prError)
}

func enrichPRsError(err error, repositoryByAlias map[string]models.Repository) error {
	var repoError repositoryError
	if errors.As(err, &repoError) {
		if repository, isKnownAlias := repositoryByAlias[repoError.alias]; isKnownAlias {
			return fmt.Errorf(
				"error fetching reviews and comments from %s/%s: %w",
				repository.Owner, repository.Name, err,
			)
		}
	}
	return fmt.Errorf("error fetching reviews and comments: %w", err)
}

func pullRequestRefOfResult(result PRResult) models.PullRequestRef {
	return models.PullRequestRef{Repository: result.repository, Number: result.pr.GetNumber()}
}

func enrichedPR(result PRResult, aliasNode *pullRequestWrapperNode, aliasError error) PR {
	node, isEnriched := enrichedNode(aliasNode)
	if !isEnriched {
		aliasError = cmp.Or(aliasError, errNoPullRequestReturned)
	}
	logEnrichment(result.repository, result.pr.GetNumber(), node, aliasError)
	return prWithReviewers(result.pr, result.repository, node)
}
