package githubclient

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
	"github.com/hellej/pr-slack-reminder-action/testhelpers/retryhelpers"
)

func TestBuildCheckStatusQuery(t *testing.T) {
	references := []models.PullRequestRef{
		{Repository: testRepositories[0], Number: 111},
		{Repository: testRepositories[1], Number: 222},
	}

	query := buildPullRequestsQuery(references, checkStatusSelection)

	expectedText := "query($owner0:String!,$name0:String!,$num0:Int!,$owner1:String!,$name1:String!,$num1:Int!){\n" +
		"  rateLimit { cost remaining limit }\n" +
		"  p0: repository(owner:$owner0,name:$name0){ pullRequest(number:$num0){ number " +
		"statusCheckRollup { contexts(first: 100) { pageInfo { hasNextPage } nodes { __typename " +
		"... on CheckRun { status conclusion isRequired(pullRequestNumber: $num0) } " +
		"... on StatusContext { state isRequired(pullRequestNumber: $num0) } } } } } }\n" +
		"  p1: repository(owner:$owner1,name:$name1){ pullRequest(number:$num1){ number " +
		"statusCheckRollup { contexts(first: 100) { pageInfo { hasNextPage } nodes { __typename " +
		"... on CheckRun { status conclusion isRequired(pullRequestNumber: $num1) } " +
		"... on StatusContext { state isRequired(pullRequestNumber: $num1) } } } } } }\n" +
		"}\n"
	if query.text != expectedText {
		t.Errorf("query text =\n%s\nexpected\n%s", query.text, expectedText)
	}
	expectedVariables := map[string]any{
		"owner0": "owner-one", "name0": "repo-one", "num0": 111,
		"owner1": "owner-two", "name1": "repo-two", "num1": 222,
	}
	if !reflect.DeepEqual(query.variables, expectedVariables) {
		t.Errorf("variables = %+v, expected %+v", query.variables, expectedVariables)
	}
}

func requiredCheckRun(status, conclusion string) statusCheckContextNode {
	return statusCheckContextNode{
		Typename: checkRunTypename, Status: status, Conclusion: conclusion, IsRequired: true,
	}
}

func requiredStatusContext(state string) statusCheckContextNode {
	return statusCheckContextNode{Typename: statusContextTypename, State: state, IsRequired: true}
}

func optional(context statusCheckContextNode) statusCheckContextNode {
	context.IsRequired = false
	return context
}

func rollupOf(hasNextPage bool, contexts ...statusCheckContextNode) *statusCheckRollupNode {
	rollup := &statusCheckRollupNode{}
	rollup.Contexts.PageInfo.HasNextPage = hasNextPage
	rollup.Contexts.Nodes = contexts
	return rollup
}

func TestRequiredChecksStatus(t *testing.T) {
	tests := []struct {
		name     string
		rollup   *statusCheckRollupNode
		expected RequiredChecksStatus
	}{
		{"null rollup", nil, RequiredChecksUnknown},
		{"no contexts", rollupOf(false), RequiredChecksUnknown},
		{
			"only optional contexts, one failing",
			rollupOf(false, optional(requiredCheckRun("COMPLETED", "FAILURE")), optional(requiredStatusContext("SUCCESS"))),
			RequiredChecksUnknown,
		},
		{"required check run failed", rollupOf(false, requiredCheckRun("COMPLETED", "FAILURE")), RequiredChecksFailing},
		{"required check run timed out", rollupOf(false, requiredCheckRun("COMPLETED", "TIMED_OUT")), RequiredChecksFailing},
		{"required check run cancelled", rollupOf(false, requiredCheckRun("COMPLETED", "CANCELLED")), RequiredChecksFailing},
		{"required check run needs action", rollupOf(false, requiredCheckRun("COMPLETED", "ACTION_REQUIRED")), RequiredChecksFailing},
		{"required check run succeeded", rollupOf(false, requiredCheckRun("COMPLETED", "SUCCESS")), RequiredChecksPassing},
		{"required check run neutral", rollupOf(false, requiredCheckRun("COMPLETED", "NEUTRAL")), RequiredChecksPassing},
		{"required check run skipped", rollupOf(false, requiredCheckRun("COMPLETED", "SKIPPED")), RequiredChecksPassing},
		{"required check run in progress", rollupOf(false, requiredCheckRun("IN_PROGRESS", "")), RequiredChecksUnknown},
		{"required check run queued", rollupOf(false, requiredCheckRun("QUEUED", "")), RequiredChecksUnknown},
		{"required status failure", rollupOf(false, requiredStatusContext("FAILURE")), RequiredChecksFailing},
		{"required status error", rollupOf(false, requiredStatusContext("ERROR")), RequiredChecksFailing},
		{"required status success", rollupOf(false, requiredStatusContext("SUCCESS")), RequiredChecksPassing},
		{"required status pending", rollupOf(false, requiredStatusContext("PENDING")), RequiredChecksUnknown},
		{"required status expected", rollupOf(false, requiredStatusContext("EXPECTED")), RequiredChecksUnknown},
		{
			"a failing required context among pending and passing ones",
			rollupOf(false,
				requiredCheckRun("COMPLETED", "SUCCESS"),
				requiredCheckRun("IN_PROGRESS", ""),
				requiredStatusContext("ERROR"),
			),
			RequiredChecksFailing,
		},
		{
			"a pending required context beside a passing one",
			rollupOf(false, requiredStatusContext("SUCCESS"), requiredCheckRun("IN_PROGRESS", "")),
			RequiredChecksUnknown,
		},
		{
			"every required context passing, an optional one failing",
			rollupOf(false,
				requiredCheckRun("COMPLETED", "SUCCESS"),
				requiredStatusContext("SUCCESS"),
				optional(requiredCheckRun("COMPLETED", "FAILURE")),
				optional(requiredStatusContext("FAILURE")),
			),
			RequiredChecksPassing,
		},
		{
			"every required context passing, more contexts on the next page",
			rollupOf(true, requiredCheckRun("COMPLETED", "SUCCESS"), requiredStatusContext("SUCCESS")),
			RequiredChecksUnknown,
		},
		{
			"a failing required context, more contexts on the next page",
			rollupOf(true, requiredCheckRun("COMPLETED", "FAILURE")),
			RequiredChecksFailing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := requiredChecksStatus(tt.rollup); got != tt.expected {
				t.Errorf("requiredChecksStatus() = %q, expected %q", got, tt.expected)
			}
		})
	}
}

type checkStatusFixture struct {
	rollup        any // rendered as the PR's statusCheckRollup; nil renders null
	errorType     string
	errorPath     []any // path suffix after the alias
	requestStatus int   // non-zero: the whole request carrying this PR fails with this status
	rawAlias      string
}

// Answers check status requests from fixtures by PR number. A request carrying a number in
// waitForCancelNumbers waits until another request has been answered, then up to the given
// time for its own context to be cancelled.
type fakeCheckStatusTransport struct {
	fixtureByNumber      map[int]checkStatusFixture
	waitForCancelNumbers []int
	waitForCancel        time.Duration
	answered             chan struct{}
	mutex                sync.Mutex
	requestedNumbers     [][]int
}

func (t *fakeCheckStatusTransport) Post(ctx context.Context, body []byte) (int, json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return 0, nil, err
	}
	var request graphqlRequest
	if err := json.Unmarshal(body, &request); err != nil {
		return 0, nil, err
	}
	numbers := postedPRNumbers(request.Variables)

	t.mutex.Lock()
	t.requestedNumbers = append(t.requestedNumbers, numbers)
	t.mutex.Unlock()

	if slices.ContainsFunc(numbers, func(number int) bool { return slices.Contains(t.waitForCancelNumbers, number) }) {
		<-t.answered
		select {
		case <-ctx.Done():
			return 0, nil, ctx.Err()
		case <-time.After(t.waitForCancel):
		}
	} else if t.answered != nil {
		defer close(t.answered)
	}

	aliasTexts := []string{`"rateLimit":{"cost":1,"remaining":4999,"limit":5000}`}
	responseErrors := []map[string]any{}
	for index, number := range numbers {
		fixture := t.fixtureByNumber[number]
		if fixture.requestStatus != 0 {
			return fixture.requestStatus, json.RawMessage(`{"message":"forbidden"}`), nil
		}
		alias := fmt.Sprintf("p%d", index)
		aliasTexts = append(aliasTexts, fmt.Sprintf("%q:%s", alias, fixture.aliasJSON(number)))
		if fixture.errorType != "" {
			responseErrors = append(responseErrors, map[string]any{
				"type": fixture.errorType, "path": append([]any{alias}, fixture.errorPath...), "message": "failed",
			})
		}
	}
	errorsJSON, err := json.Marshal(responseErrors)
	if err != nil {
		return 0, nil, err
	}
	return 200, json.RawMessage(fmt.Sprintf(
		`{"data":{%s},"errors":%s}`, strings.Join(aliasTexts, ","), errorsJSON,
	)), nil
}

func (f checkStatusFixture) aliasJSON(number int) string {
	if f.rawAlias != "" {
		return f.rawAlias
	}
	if len(f.errorPath) == 1 {
		return `{"pullRequest":null}`
	}
	rollupJSON, _ := json.Marshal(f.rollup)
	return fmt.Sprintf(`{"pullRequest":{"number":%d,"statusCheckRollup":%s}}`, number, rollupJSON)
}

func (t *fakeCheckStatusTransport) requested() [][]int {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return slices.Clone(t.requestedNumbers)
}

func rollupJSON(contexts ...map[string]any) map[string]any {
	return map[string]any{"contexts": map[string]any{
		"pageInfo": map[string]any{"hasNextPage": false}, "nodes": contexts,
	}}
}

func checkRunJSON(status, conclusion string) map[string]any {
	return map[string]any{
		"__typename": "CheckRun", "status": status, "conclusion": conclusion, "isRequired": true,
	}
}

var failingRollup = rollupJSON(checkRunJSON("COMPLETED", "FAILURE"))

var passingRollup = rollupJSON(checkRunJSON("COMPLETED", "SUCCESS"))

func approvedPR(number int) PR {
	return PR{
		PullRequest:             &PullRequest{Number: number},
		Repository:              testRepositories[0],
		ApprovedByUsers:         []Collaborator{{Login: "approver"}},
		HasCompleteReviewerInfo: true,
	}
}

func approvedPRs(firstNumber, count int) []PR {
	prs := make([]PR, count)
	for index := range prs {
		prs[index] = approvedPR(firstNumber + index)
	}
	return prs
}

func requiredChecksOf(prs []PR) []RequiredChecksStatus {
	return utilities.Map(prs, func(pr PR) RequiredChecksStatus { return pr.RequiredChecks })
}

func newCheckStatusTestClient(transport graphqlTransport) *client {
	retryPolicy, _ := retryhelpers.SkipAndRecordWaits()
	return &client{graphql: graphqlClient{transport: transport, retryPolicy: retryPolicy}}
}

func TestFetchRequiredCheckStatusRequestsOnlyApprovedNonDraftPRsWithCompleteReviewerInfo(t *testing.T) {
	draft := approvedPR(2)
	draft.PullRequest.Draft = true
	unapproved := approvedPR(3)
	unapproved.ApprovedByUsers = nil
	unapproved.CommentedByUsers = []Collaborator{{Login: "commenter"}}
	incomplete := approvedPR(4)
	incomplete.HasCompleteReviewerInfo = false
	prs := []PR{approvedPR(1), draft, unapproved, incomplete, approvedPR(5)}
	fixtureByNumber := map[int]checkStatusFixture{}
	for _, pr := range prs {
		fixtureByNumber[pr.Number] = checkStatusFixture{rollup: failingRollup}
	}
	transport := &fakeCheckStatusTransport{fixtureByNumber: fixtureByNumber}

	result, err := newCheckStatusTestClient(transport).fetchRequiredCheckStatus(context.Background(), prs)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := transport.requested(); !reflect.DeepEqual(got, [][]int{{1, 5}}) {
		t.Errorf("requested PR numbers = %v, expected [[1 5]]", got)
	}
	expected := []RequiredChecksStatus{
		RequiredChecksFailing, RequiredChecksUnknown, RequiredChecksUnknown, RequiredChecksUnknown,
		RequiredChecksFailing,
	}
	if got := requiredChecksOf(result); !reflect.DeepEqual(got, expected) {
		t.Errorf("RequiredChecks = %q, expected %q", got, expected)
	}
}

func TestFetchRequiredCheckStatusSendsNoRequestWithoutAnApprovedPR(t *testing.T) {
	unapproved := approvedPR(1)
	unapproved.ApprovedByUsers = nil
	transport := &fakeCheckStatusTransport{}

	result, err := newCheckStatusTestClient(transport).fetchRequiredCheckStatus(
		context.Background(), []PR{unapproved},
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := transport.requested(); len(got) != 0 {
		t.Errorf("requested PR numbers = %v, expected no request", got)
	}
	if len(result) != 1 {
		t.Errorf("got %d PRs, expected the 1 given", len(result))
	}
}

func TestFetchRequiredCheckStatusBatches25PRsPerRequest(t *testing.T) {
	prs := approvedPRs(1, 26)
	transport := &fakeCheckStatusTransport{fixtureByNumber: map[int]checkStatusFixture{
		1: {rollup: failingRollup}, 25: {rollup: passingRollup}, 26: {rollup: failingRollup},
	}}

	result, err := newCheckStatusTestClient(transport).fetchRequiredCheckStatus(context.Background(), prs)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	requestSizes := utilities.Map(transport.requested(), func(numbers []int) int { return len(numbers) })
	slices.Sort(requestSizes)
	if !reflect.DeepEqual(requestSizes, []int{1, 25}) {
		t.Errorf("request sizes = %v, expected [1 25]", requestSizes)
	}
	statuses := requiredChecksOf(result)
	if statuses[0] != RequiredChecksFailing || statuses[24] != RequiredChecksPassing ||
		statuses[25] != RequiredChecksFailing || statuses[1] != RequiredChecksUnknown {
		t.Errorf("RequiredChecks = %q, expected 1 and 26 failing, 25 passing, 2 unknown", statuses)
	}
}

// The second batch waits for the first to fail, so a failure cancelling the shared context
// would fail it too.
func TestFetchRequiredCheckStatusBatchesFailIndependently(t *testing.T) {
	prs := approvedPRs(1, 26)
	transport := &fakeCheckStatusTransport{
		fixtureByNumber: map[int]checkStatusFixture{
			1:  {requestStatus: 403},
			26: {rollup: failingRollup},
		},
		waitForCancelNumbers: []int{26},
		waitForCancel:        100 * time.Millisecond,
		answered:             make(chan struct{}),
	}

	result, err := newCheckStatusTestClient(transport).fetchRequiredCheckStatus(context.Background(), prs)

	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("error = %v, expected the first batch's 403", err)
	}
	statuses := requiredChecksOf(result)
	if !slices.Equal(statuses[:25], slices.Repeat([]RequiredChecksStatus{RequiredChecksUnknown}, 25)) {
		t.Errorf("first batch RequiredChecks = %q, expected every one unknown", statuses[:25])
	}
	if statuses[25] != RequiredChecksFailing {
		t.Errorf("PR 26 RequiredChecks = %q, expected failing from the second batch", statuses[25])
	}
}

func TestFetchRequiredCheckStatusPerPRErrorLeavesOnlyThatPRUnknown(t *testing.T) {
	tests := []struct {
		name    string
		fixture checkStatusFixture
	}{
		{"pull request error", checkStatusFixture{errorType: "NOT_FOUND", errorPath: []any{"pullRequest"}}},
		{
			"field error under the pull request",
			checkStatusFixture{errorType: "FORBIDDEN", errorPath: []any{"pullRequest", "statusCheckRollup"}},
		},
		{"pull request missing without an error", checkStatusFixture{rawAlias: `{"pullRequest":null}`}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeCheckStatusTransport{fixtureByNumber: map[int]checkStatusFixture{
				1: {rollup: failingRollup}, 2: tt.fixture, 3: {rollup: passingRollup},
			}}

			result, err := newCheckStatusTestClient(transport).fetchRequiredCheckStatus(
				context.Background(), approvedPRs(1, 3),
			)

			if err == nil || !strings.Contains(err.Error(), "owner-one/repo-one/2") {
				t.Errorf("error = %v, expected one naming PR owner-one/repo-one/2", err)
			}
			expected := []RequiredChecksStatus{RequiredChecksFailing, RequiredChecksUnknown, RequiredChecksPassing}
			if got := requiredChecksOf(result); !reflect.DeepEqual(got, expected) {
				t.Errorf("RequiredChecks = %q, expected %q", got, expected)
			}
		})
	}
}

func TestFetchRequiredCheckStatusFailedBatchLeavesEveryPRUnknown(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	tests := []struct {
		name    string
		ctx     context.Context
		fixture checkStatusFixture
	}{
		{"repository error", context.Background(), checkStatusFixture{errorType: "FORBIDDEN", errorPath: []any{}}},
		{
			"decode error",
			context.Background(),
			checkStatusFixture{rawAlias: `{"pullRequest":{"number":2,"statusCheckRollup":"not an object"}}`},
		},
		{"expired context", cancelledCtx, checkStatusFixture{rollup: failingRollup}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeCheckStatusTransport{fixtureByNumber: map[int]checkStatusFixture{
				1: {rollup: failingRollup}, 2: tt.fixture,
			}}

			result, err := newCheckStatusTestClient(transport).fetchRequiredCheckStatus(tt.ctx, approvedPRs(1, 2))

			if err == nil {
				t.Error("expected an error")
			}
			expected := []RequiredChecksStatus{RequiredChecksUnknown, RequiredChecksUnknown}
			if got := requiredChecksOf(result); !reflect.DeepEqual(got, expected) {
				t.Errorf("RequiredChecks = %q, expected %q", got, expected)
			}
		})
	}
}

func TestFetchRequiredCheckStatusLogs(t *testing.T) {
	logOutput := captureLogOutput(t)
	truncatedRollup := rollupJSON(checkRunJSON("COMPLETED", "SUCCESS"))
	truncatedRollup["contexts"].(map[string]any)["pageInfo"] = map[string]any{"hasNextPage": true}
	transport := &fakeCheckStatusTransport{fixtureByNumber: map[int]checkStatusFixture{
		1:  {rollup: failingRollup},
		2:  {rollup: truncatedRollup},
		3:  {rollup: passingRollup},
		26: {requestStatus: 403},
	}}

	_, _ = newCheckStatusTestClient(transport).fetchRequiredCheckStatus(context.Background(), approvedPRs(1, 26))

	for _, expected := range []string{
		"GraphQL rate limit: cost 1, remaining 4999 of 5000",
		"Required check status of PR owner-one/repo-one/1: failing",
		"Required check status of PR owner-one/repo-one/2: unknown",
		"Required check status of PR owner-one/repo-one/3: passing",
		"PR owner-one/repo-one/2 has more than 100 check contexts",
		"Unable to fetch required check status of PRs owner-one/repo-one/26:",
		"403",
	} {
		if !strings.Contains(logOutput.String(), expected) {
			t.Errorf("log output is missing %q, got:\n%s", expected, logOutput.String())
		}
	}
}
