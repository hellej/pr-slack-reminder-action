package githubclient

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log"
	"maps"
	"slices"
	"strings"

	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
	"golang.org/x/sync/errgroup"
)

// Whether a PR's required checks block its merge. See githubclient.spec.md § Behaviour
type RequiredChecksStatus string

const (
	RequiredChecksUnknown RequiredChecksStatus = ""
	RequiredChecksPassing RequiredChecksStatus = "passing"
	RequiredChecksFailing RequiredChecksStatus = "failing"
)

const (
	checkRunTypename      = "CheckRun"
	statusContextTypename = "StatusContext"
)

const completedCheckRunStatus = "COMPLETED"

var passingCheckRunConclusions = []string{"SUCCESS", "NEUTRAL", "SKIPPED"}

var failingStatusContextStates = []string{"FAILURE", "ERROR"}

const successStatusContextState = "SUCCESS"

// A CheckRun carries status and conclusion, a StatusContext carries state.
type statusCheckContextNode struct {
	Typename   string `json:"__typename"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	State      string `json:"state"`
	IsRequired bool   `json:"isRequired"`
}

type statusCheckRollupNode struct {
	Contexts struct {
		PageInfo struct {
			HasNextPage bool `json:"hasNextPage"`
		} `json:"pageInfo"`
		Nodes []statusCheckContextNode `json:"nodes"`
	} `json:"contexts"`
}

type checkStatusPullRequestNode struct {
	StatusCheckRollup *statusCheckRollupNode `json:"statusCheckRollup"`
}

type checkStatusWrapperNode struct {
	PullRequest *checkStatusPullRequestNode `json:"pullRequest"`
}

// isRequired takes the PR number as an argument, so the selection is inline per alias.
var checkStatusSelection = pullRequestSelection{
	body: func(numberVariable string) string {
		return fmt.Sprintf(
			"number statusCheckRollup { contexts(first: 100) { pageInfo { hasNextPage } nodes { __typename "+
				"... on CheckRun { status conclusion isRequired(pullRequestNumber: %[1]s) } "+
				"... on StatusContext { state isRequired(pullRequestNumber: %[1]s) } } } }",
			numberVariable,
		)
	},
}

func classifyRequiredChecks(rollup *statusCheckRollupNode) RequiredChecksStatus {
	if rollup == nil {
		return RequiredChecksUnknown
	}
	requiredContexts := utilities.Filter(
		rollup.Contexts.Nodes, func(node statusCheckContextNode) bool { return node.IsRequired },
	)
	if slices.ContainsFunc(requiredContexts, statusCheckContextNode.isFailing) {
		return RequiredChecksFailing
	}
	passingContexts := utilities.Filter(requiredContexts, statusCheckContextNode.isPassing)
	everyRequiredContextPassed := len(requiredContexts) > 0 && len(passingContexts) == len(requiredContexts)
	if everyRequiredContextPassed && !rollup.Contexts.PageInfo.HasNextPage {
		return RequiredChecksPassing
	}
	return RequiredChecksUnknown
}

func (n statusCheckContextNode) isFailing() bool {
	switch n.Typename {
	case checkRunTypename:
		return n.Status == completedCheckRunStatus && !slices.Contains(passingCheckRunConclusions, n.Conclusion)
	case statusContextTypename:
		return slices.Contains(failingStatusContextStates, n.State)
	}
	return false
}

func (n statusCheckContextNode) isPassing() bool {
	switch n.Typename {
	case checkRunTypename:
		return n.Status == completedCheckRunStatus && slices.Contains(passingCheckRunConclusions, n.Conclusion)
	case statusContextTypename:
		return n.State == successStatusContextState
	}
	return false
}

// Only an approved PR can reach "ready to merge", which a failing required check blocks.
func needsRequiredCheckStatus(pr PR) bool {
	return !pr.GetDraft() && pr.HasCompleteReviewerInfo && len(pr.ApprovedByUsers) > 0
}

// Returns the PRs, in input order, with RequiredChecks set on those needing it. A failed batch
// leaves its PRs unknown and fails no other batch. The returned error joins every failure.
func (c *client) fetchRequiredCheckStatus(ctx context.Context, prs []PR) ([]PR, error) {
	requestedRefs := utilities.Map(utilities.Filter(prs, needsRequiredCheckStatus), PR.GetPullRequestRef)
	if len(requestedRefs) == 0 {
		return prs, nil
	}
	log.Printf("\nFetching required check status for %d approved pull requests", len(requestedRefs))

	batches := slices.Collect(slices.Chunk(requestedRefs, enrichBatchSize))
	statusByRefOfBatch := make([]map[models.PullRequestRef]RequiredChecksStatus, len(batches))
	batchErrors := make([]error, len(batches))

	var batchGroup errgroup.Group
	batchGroup.SetLimit(defaultGitHubAPIConcurrencyLimit)
	for index, batch := range batches {
		batchGroup.Go(func() error {
			statusByRefOfBatch[index], batchErrors[index] = c.fetchRequiredCheckStatusBatch(ctx, batch)
			return nil
		})
	}
	_ = batchGroup.Wait()

	statusByRef := map[models.PullRequestRef]RequiredChecksStatus{}
	for _, batchStatusByRef := range statusByRefOfBatch {
		maps.Copy(statusByRef, batchStatusByRef)
	}
	return utilities.Map(prs, func(pr PR) PR {
		pr.RequiredChecks = statusByRef[pr.GetPullRequestRef()]
		return pr
	}), errors.Join(batchErrors...)
}

// A failed batch returns no statuses. A PR-scoped error leaves only that PR out.
func (c *client) fetchRequiredCheckStatusBatch(
	ctx context.Context, references []models.PullRequestRef,
) (map[models.PullRequestRef]RequiredChecksStatus, error) {
	query := buildPullRequestsQuery(references, checkStatusSelection)

	var data aliasedData[checkStatusWrapperNode]
	fieldErrors, err := c.graphql.Do(ctx, query.text, query.variables, query.aliases, &data)
	if err != nil && !failsOnlyOnePullRequest(err) {
		log.Printf(
			"Unable to fetch required check status of PRs %s: %v", pullRequestPathsText(references), err,
		)
		return nil, pullRequestBatchError("required check status", err, query.repositoryByAlias)
	}
	logRateLimit(data.RateLimit)

	statusByRef := map[models.PullRequestRef]RequiredChecksStatus{}
	pullRequestErrors := []error{}
	for index, reference := range references {
		alias := query.aliases[index]
		pullRequest, aliasError := checkStatusPullRequest(data.ByAlias[alias], errorsForAlias(alias, err, fieldErrors))
		if aliasError != nil {
			pullRequestError := fmt.Errorf(
				"error fetching required check status of PR %s: %w", pullRequestPath(reference), aliasError,
			)
			log.Print(pullRequestError)
			pullRequestErrors = append(pullRequestErrors, pullRequestError)
			continue
		}
		statusByRef[reference] = classifyRequiredChecks(pullRequest.StatusCheckRollup)
		logRequiredCheckStatus(reference, pullRequest.StatusCheckRollup, statusByRef[reference])
	}
	return statusByRef, errors.Join(pullRequestErrors...)
}

func checkStatusPullRequest(
	aliasNode *checkStatusWrapperNode, aliasError error,
) (*checkStatusPullRequestNode, error) {
	if aliasError != nil {
		return nil, aliasError
	}
	if aliasNode == nil || aliasNode.PullRequest == nil {
		return nil, errNoPullRequestReturned
	}
	return aliasNode.PullRequest, nil
}

func logRequiredCheckStatus(
	reference models.PullRequestRef, rollup *statusCheckRollupNode, status RequiredChecksStatus,
) {
	if rollup != nil && rollup.Contexts.PageInfo.HasNextPage {
		log.Printf(
			"PR %s has more than 100 check contexts, only the first 100 are read",
			pullRequestPath(reference),
		)
	}
	log.Printf(
		"Required check status of PR %s: %s", pullRequestPath(reference), cmp.Or(string(status), "unknown"),
	)
}

func pullRequestPath(reference models.PullRequestRef) string {
	return fmt.Sprintf("%s/%d", reference.Repository.GetPath(), reference.Number)
}

func pullRequestPathsText(references []models.PullRequestRef) string {
	return strings.Join(utilities.Map(references, pullRequestPath), ", ")
}
