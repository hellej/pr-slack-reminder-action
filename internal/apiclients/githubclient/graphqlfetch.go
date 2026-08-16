package githubclient

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
	"golang.org/x/sync/errgroup"
)

const openPullRequestState = "open"

const rateLimitField = "rateLimit"

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

type graphqlQuery struct {
	text              string
	variables         map[string]any
	aliases           []string
	repositoryByAlias map[string]models.Repository
}

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

func assembleQuery(variableDeclarations, aliasSelections []string, fragment string) string {
	return fmt.Sprintf(
		"query(%s){\n  %s { cost remaining limit }\n%s\n}\n%s",
		strings.Join(variableDeclarations, ","),
		rateLimitField,
		strings.Join(aliasSelections, "\n"),
		fragment,
	)
}

type rateLimitNode struct {
	Cost      int `json:"cost"`
	Remaining int `json:"remaining"`
	Limit     int `json:"limit"`
}

type repositoryPullRequestsNode struct {
	PullRequests connection[pullRequestNode] `json:"pullRequests"`
}

// The aliases are generated, so they are decoded by name rather than by struct field.
type aliasedData[T any] struct {
	RateLimit rateLimitNode
	ByAlias   map[string]*T
}

func (d *aliasedData[T]) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}

	d.ByAlias = map[string]*T{}
	for field, value := range fields {
		if field == rateLimitField {
			if err := json.Unmarshal(value, &d.RateLimit); err != nil {
				return err
			}
			continue
		}
		var node *T
		if err := json.Unmarshal(value, &node); err != nil {
			return err
		}
		d.ByAlias[field] = node
	}
	return nil
}

func (c *client) listOpenPRs(
	ctx context.Context, repositories []models.Repository,
) ([]PRResult, error) {
	query := buildListOpenPRsQuery(repositories)

	callCtx, cancel := context.WithTimeout(ctx, PullRequestListTimeout)
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

func logRateLimit(rateLimit rateLimitNode) {
	log.Printf(
		"GraphQL rate limit: cost %d, remaining %d of %d",
		rateLimit.Cost, rateLimit.Remaining, rateLimit.Limit,
	)
}

func openPRResultMapper(repository models.Repository) func(node pullRequestNode) PRResult {
	return func(node pullRequestNode) PRResult {
		return PRResult{pr: openPullRequestFromNode(node), repository: repository}
	}
}

// states: OPEN guarantees the state, which is therefore not selected.
func openPullRequestFromNode(node pullRequestNode) *PullRequest {
	return &PullRequest{
		Number:    node.Number,
		Title:     node.Title,
		HTMLURL:   node.URL,
		CreatedAt: node.CreatedAt,
		UpdatedAt: node.UpdatedAt,
		State:     openPullRequestState,
		Merged:    false,
		Draft:     node.IsDraft,
		Labels:    utilities.Map(node.Labels.Nodes, func(label labelNode) string { return label.Name }),
		Author:    collaboratorFromAuthorNode(node.Author),
		HeadSHA:   node.HeadRefOID,
	}
}

const enrichBatchSize = 25

const approvedReviewState = "APPROVED"

// A pending review is visible only to its own author, so it contributes no reviewer.
const pendingReviewState = "PENDING"

const enrichedPullRequestFragmentName = "enrichedPr"

// commits are selected for the PR tracker canvas and are not read yet.
const enrichedPullRequestFragment = `fragment ` + enrichedPullRequestFragmentName + ` on PullRequest {
  number
  commits(last: 1){ nodes { commit { oid committedDate } } }
  reviews(first: 100){ nodes { state author { login __typename ... on User { name } } } }
  comments(first: 100){ nodes { createdAt body author { login __typename ... on User { name } } } }
}`

func buildEnrichPRsQuery(references []models.PullRequestRef) graphqlQuery {
	query := graphqlQuery{
		variables:         map[string]any{},
		aliases:           make([]string, len(references)),
		repositoryByAlias: map[string]models.Repository{},
	}
	variableDeclarations := make([]string, 0, 3*len(references))
	pullRequestSelections := make([]string, len(references))

	for index, reference := range references {
		alias := fmt.Sprintf("p%d", index)
		ownerVariable := fmt.Sprintf("owner%d", index)
		nameVariable := fmt.Sprintf("name%d", index)
		numberVariable := fmt.Sprintf("num%d", index)

		query.aliases[index] = alias
		query.repositoryByAlias[alias] = reference.Repository
		query.variables[ownerVariable] = reference.Repository.Owner
		query.variables[nameVariable] = reference.Repository.Name
		query.variables[numberVariable] = reference.Number
		variableDeclarations = append(
			variableDeclarations,
			"$"+ownerVariable+":String!", "$"+nameVariable+":String!", "$"+numberVariable+":Int!",
		)
		pullRequestSelections[index] = fmt.Sprintf(
			"  %s: repository(owner:$%s,name:$%s){ pullRequest(number:$%s){ ...%s } }",
			alias, ownerVariable, nameVariable, numberVariable, enrichedPullRequestFragmentName,
		)
	}

	query.text = assembleQuery(variableDeclarations, pullRequestSelections, enrichedPullRequestFragment)
	return query
}

type pullRequestWrapperNode struct {
	PullRequest *pullRequestNode `json:"pullRequest"`
}

// Fetches reviews and comments for the given PRs and returns them enriched, in input order.
// Enrichment failures scoped to one PR are logged and leave that PR without reviewer info.
func (c *client) enrichPRs(ctx context.Context, prResults []PRResult) ([]PR, error) {
	log.Printf("\nFetching pull request reviews and comments for PRs")

	batches := slices.Collect(slices.Chunk(prResults, enrichBatchSize))
	enrichedBatches := make([][]PR, len(batches))

	batchGroup, batchCtx := errgroup.WithContext(ctx)
	batchGroup.SetLimit(DefaultGitHubAPIConcurrencyLimit)

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
	query := buildEnrichPRsQuery(utilities.Map(batch, pullRequestRefOfResult))

	callCtx, cancel := context.WithTimeout(ctx, ReviewsFetchTimeout)
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

func errorsForAlias(alias string, hardError error, fieldErrors []fieldError) error {
	aliasErrors := []error{}

	var prError pullRequestError
	if errors.As(hardError, &prError) && prError.alias == alias {
		aliasErrors = append(aliasErrors, prError)
	}
	for _, err := range fieldErrors {
		if err.alias == alias {
			aliasErrors = append(aliasErrors, err)
		}
	}
	return errors.Join(aliasErrors...)
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

var errNoPullRequestReturned = errors.New("no pull request returned")

func enrichedPR(result PRResult, aliasNode *pullRequestWrapperNode, aliasError error) PR {
	node, isEnriched := enrichedNode(aliasNode)
	if !isEnriched {
		aliasError = cmp.Or(aliasError, errNoPullRequestReturned)
	}
	logEnrichment(result, node, aliasError)

	submittedReviews := utilities.Filter(node.Reviews.Nodes, isSubmittedUserReview)
	approvingReviews := utilities.Filter(submittedReviews, isApprovingReviewNode)
	commentsFromUsers := utilities.Filter(node.Comments.Nodes, hasValidCommentAuthor)
	timelineComments := utilities.Map(node.Comments.Nodes, timelineCommentFromNode)

	approvedByUsers, commentedByUsers := deriveReviewers(
		result.pr.Author.Login,
		utilities.Map(approvingReviews, reviewAuthor),
		utilities.Map(submittedReviews, reviewAuthor),
		nil, // review comment authors are always review authors, so review comments are not fetched

		utilities.Map(commentsFromUsers, commentAuthor),
	)

	return PR{
		PullRequest:      result.pr,
		Repository:       result.repository,
		Author:           result.pr.Author,
		ApprovedByUsers:  approvedByUsers,
		CommentedByUsers: commentedByUsers,
		SnoozedUntil:     findActiveSnooze(timelineComments),
	}
}

func enrichedNode(aliasNode *pullRequestWrapperNode) (pullRequestNode, bool) {
	if aliasNode == nil || aliasNode.PullRequest == nil {
		return pullRequestNode{}, false
	}
	return *aliasNode.PullRequest, true
}

func logEnrichment(result PRResult, node pullRequestNode, err error) {
	if err != nil {
		log.Printf("Unable to fetch reviews/comments for PR #%d: %v", result.pr.GetNumber(), err)
		return
	}
	log.Printf(
		"Found %d reviews and %d timeline comments for PR %v/%d",
		len(node.Reviews.Nodes), len(node.Comments.Nodes), result.repository, result.pr.GetNumber(),
	)
}

func isSubmittedUserReview(review reviewNode) bool {
	return review.State != pendingReviewState && hasValidAuthorNode(review.Author)
}

func isApprovingReviewNode(review reviewNode) bool {
	return review.State == approvedReviewState
}

func reviewAuthor(review reviewNode) Collaborator {
	return collaboratorFromAuthorNode(review.Author)
}

func hasValidCommentAuthor(comment commentNode) bool {
	return hasValidAuthorNode(comment.Author)
}

func commentAuthor(comment commentNode) Collaborator {
	return collaboratorFromAuthorNode(comment.Author)
}
