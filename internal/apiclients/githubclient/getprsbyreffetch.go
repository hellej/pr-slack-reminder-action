package githubclient

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
	"golang.org/x/sync/errgroup"
)

const notFoundErrorType = "NOT_FOUND"

// The referenced PRs may be closed or merged, so state and merged are selected alongside the
// scalars, author and labels the open-PR listing provides in the "post" run mode.
const fullPullRequestSelection = `  number title url isDraft createdAt updatedAt state merged mergedAt
  author { login __typename ... on User { name } }
  labels(first: 100){ nodes { name } }
  reviews(first: 100){ nodes { state author { login __typename ... on User { name } } } }
  comments(first: 100){ nodes { createdAt body author { login __typename ... on User { name } } } }`

var fullPullRequestFragment = newPullRequestFragment("fullPr", fullPullRequestSelection)

// Fetches the referenced PRs with their reviews and comments, in input order. A PR that cannot
// be fetched fails the call: unlike in FindOpenPRs there are no listed scalars to fall back to.
func (c *client) getPRsByRef(ctx context.Context, references []models.PullRequestRef) ([]PR, error) {
	batches := slices.Collect(slices.Chunk(references, enrichBatchSize))
	fetchedBatches := make([][]PR, len(batches))

	batchGroup, batchCtx := errgroup.WithContext(ctx)
	batchGroup.SetLimit(defaultGitHubAPIConcurrencyLimit)

	for index, batch := range batches {
		batchGroup.Go(func() error {
			prs, err := c.getPRBatchByRef(batchCtx, batch)
			fetchedBatches[index] = prs
			return err
		})
	}
	if err := batchGroup.Wait(); err != nil {
		return nil, err
	}
	return utilities.FlatMap(fetchedBatches), nil
}

func (c *client) getPRBatchByRef(
	ctx context.Context, references []models.PullRequestRef,
) ([]PR, error) {
	query := buildPullRequestsQuery(references, fullPullRequestFragment)

	callCtx, cancel := context.WithTimeout(ctx, reviewsFetchTimeout)
	defer cancel()

	var data aliasedData[pullRequestWrapperNode]
	fieldErrors, err := c.graphql.Do(callCtx, query.text, query.variables, query.aliases, &data)
	if err != nil {
		return nil, getPRsError(err, query.referenceByAlias)
	}
	logRateLimit(data.RateLimit)

	prs := make([]PR, len(references))
	for index, reference := range references {
		alias := query.aliases[index]
		node, isFetched := enrichedNode(data.ByAlias[alias])
		if !isFetched {
			return nil, pullRequestFetchError(reference, errNoPullRequestReturned)
		}
		logEnrichment(
			reference.Repository, reference.Number, node, errorsForAlias(alias, nil, fieldErrors),
		)
		prs[index] = prWithReviewers(pullRequestFromNode(node), reference.Repository, node)
	}
	return prs, nil
}

// Reports a missing PR separately from any other fetch failure.
func getPRsError(err error, referenceByAlias map[string]models.PullRequestRef) error {
	var prError pullRequestError
	if !errors.As(err, &prError) {
		return fmt.Errorf("error fetching pull requests: %w", err)
	}
	reference, isKnownAlias := referenceByAlias[prError.alias]
	if !isKnownAlias {
		return fmt.Errorf("error fetching pull requests: %w", err)
	}
	if prError.errorType == notFoundErrorType {
		return pullRequestNotFoundError(reference)
	}
	return pullRequestFetchError(reference, err)
}

func pullRequestNotFoundError(reference models.PullRequestRef) error {
	return fmt.Errorf(
		"PR %s/%s/%d not found - check the path and permissions",
		reference.Repository.Owner, reference.Repository.Name, reference.Number,
	)
}

func pullRequestFetchError(reference models.PullRequestRef, err error) error {
	return fmt.Errorf(
		"error fetching pull request %s/%s/%d: %w",
		reference.Repository.Owner, reference.Repository.Name, reference.Number, err,
	)
}
