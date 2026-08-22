package githubclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/internal/models"
)

var testRepositories = []models.Repository{
	{Owner: "owner-one", Name: "repo-one"},
	{Owner: "owner-two", Name: "repo-two"},
}

const testFragmentName = "prs"

func TestBuildListOpenPRsQuery(t *testing.T) {
	query := buildListOpenPRsQuery(testRepositories)

	requiredFragments := []string{
		"query($owner0:String!,$name0:String!,$owner1:String!,$name1:String!)",
		"rateLimit { cost remaining limit }",
		"r0: repository(owner:$owner0,name:$name0){ ..." + testFragmentName + " }",
		"r1: repository(owner:$owner1,name:$name1){ ..." + testFragmentName + " }",
		"fragment " + testFragmentName + " on Repository {",
		"pullRequests(states: OPEN, first: 100, orderBy: {field: CREATED_AT, direction: DESC})",
		"number title url isDraft createdAt updatedAt headRefOid",
		"author { login __typename ... on User { name } }",
		"labels(first: 100) { nodes { name } }",
	}
	for _, fragment := range requiredFragments {
		if !strings.Contains(query.text, fragment) {
			t.Errorf("query text is missing %q, got:\n%s", fragment, query.text)
		}
	}

	if aliasCount := strings.Count(query.text, ": repository(owner:"); aliasCount != len(testRepositories) {
		t.Errorf("query text has %d aliased repositories, expected %d", aliasCount, len(testRepositories))
	}
	spreadCount := strings.Count(query.text, "..."+testFragmentName+" }")
	if spreadCount != len(testRepositories) {
		t.Errorf("query text spreads the fragment %d times, expected %d", spreadCount, len(testRepositories))
	}

	for _, literal := range []string{"owner-one", "owner-two", "repo-one", "repo-two"} {
		if strings.Contains(query.text, literal) {
			t.Errorf("query text contains %q, owner and name belong in the variables", literal)
		}
	}

	expectedVariables := map[string]any{
		"owner0": "owner-one",
		"name0":  "repo-one",
		"owner1": "owner-two",
		"name1":  "repo-two",
	}
	if !reflect.DeepEqual(query.variables, expectedVariables) {
		t.Errorf("variables = %+v, expected %+v", query.variables, expectedVariables)
	}

	if !reflect.DeepEqual(query.aliases, []string{"r0", "r1"}) {
		t.Errorf("aliases = %v, expected [r0 r1]", query.aliases)
	}
	if !reflect.DeepEqual(query.repositoryByAlias["r1"], testRepositories[1]) {
		t.Errorf("repositoryByAlias[r1] = %+v, expected %+v", query.repositoryByAlias["r1"], testRepositories[1])
	}
}

func pullRequestNodeJSON(number int, title string) string {
	return fmt.Sprintf(
		`{"number":%d,"title":%q,"url":"https://github.com/owner-one/repo-one/pull/%d","isDraft":false,`+
			`"createdAt":"2026-05-01T12:00:00Z","updatedAt":"2026-05-02T12:00:00Z","headRefOid":"abc123",`+
			`"author":{"login":"user1","__typename":"User","name":"User One"},`+
			`"labels":{"nodes":[{"name":"bug"}]}}`,
		number, title, number,
	)
}

func TestListOpenPRs(t *testing.T) {
	withoutRetryDelay(t)

	notFoundMessage := "repository owner-one/repo-one not found - check the repository name and permissions"

	tests := []struct {
		name             string
		status           int
		responseBody     string
		expectedNumbers  []int
		expectedErrorMsg string
	}{
		{
			name:   "pull requests are mapped from both aliases",
			status: 200,
			responseBody: `{"data":{"rateLimit":{"cost":30,"remaining":4970,"limit":5000},` +
				`"r0":{"pullRequests":{"nodes":[` + pullRequestNodeJSON(1, "First") + `]}},` +
				`"r1":{"pullRequests":{"nodes":[` + pullRequestNodeJSON(2, "Second") + `]}}}}`,
			expectedNumbers: []int{1, 2},
		},
		{
			name:   "repository error on r0 fails with the repository name",
			status: 200,
			responseBody: `{"data":{"r0":null,"r1":{"pullRequests":{"nodes":[]}}},"errors":[{"type":"NOT_FOUND",` +
				`"path":["r0"],"message":"Could not resolve to a Repository with the name 'test-org/repo-one'."}]}`,
			expectedErrorMsg: notFoundMessage,
		},
		{
			name:   "forbidden repository fails with the same message",
			status: 200,
			responseBody: `{"data":{"r0":null,"r1":{"pullRequests":{"nodes":[]}}},"errors":[{"type":"FORBIDDEN",` +
				`"path":["r0"],"message":"Resource not accessible by integration"}]}`,
			expectedErrorMsg: notFoundMessage,
		},
		{
			name:   "null alias with a matching error is not read as an empty repository",
			status: 200,
			responseBody: `{"data":{"r0":null,"r1":{"pullRequests":{"nodes":[` + pullRequestNodeJSON(2, "Second") + `]}}},` +
				`"errors":[{"type":"NOT_FOUND","path":["r0"],"message":"Could not resolve to a Repository."}]}`,
			expectedErrorMsg: notFoundMessage,
		},
		{
			name:   "whole-query error fails without naming a repository",
			status: 200,
			responseBody: `{"data":null,"errors":[{"type":"RATE_LIMITED",` +
				`"message":"API rate limit exceeded"}]}`,
			expectedErrorMsg: "error fetching pull requests: query error: RATE_LIMITED API rate limit exceeded",
		},
		{
			name:   "field error under an alias fails the list, naming the repository",
			status: 200,
			responseBody: `{"data":{"r0":{"pullRequests":{"nodes":[` + pullRequestNodeJSON(1, "First") + `]}},` +
				`"r1":{"pullRequests":{"nodes":[]}}},"errors":[{"type":"RESOURCE_LIMITS_EXCEEDED",` +
				`"path":["r1","pullRequests"],"message":"resource limit exceeded"}]}`,
			expectedErrorMsg: "error fetching pull requests from owner-two/repo-two: " +
				"field error on [r1 pullRequests]: RESOURCE_LIMITS_EXCEEDED resource limit exceeded",
		},
		{
			name:   "null alias without a matching error is not read as an empty repository",
			status: 200,
			responseBody: `{"data":{"r0":null,` +
				`"r1":{"pullRequests":{"nodes":[` + pullRequestNodeJSON(2, "Second") + `]}}}}`,
			expectedErrorMsg: "error fetching pull requests from owner-one/repo-one: no data returned",
		},
		{
			name:             "alias missing from the data is not read as an empty repository",
			status:           200,
			responseBody:     `{"data":{"r0":{"pullRequests":{"nodes":[` + pullRequestNodeJSON(1, "First") + `]}}}}`,
			expectedErrorMsg: "error fetching pull requests from owner-two/repo-two: no data returned",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &recordingTransport{status: tt.status, responseBody: tt.responseBody}
			testClient := &client{graphql: graphqlClient{transport: transport}}

			prResults, err := testClient.listOpenPRs(context.Background(), testRepositories)

			if tt.expectedErrorMsg != "" {
				if err == nil {
					t.Fatalf("expected error %q, got none with %d results", tt.expectedErrorMsg, len(prResults))
				}
				if err.Error() != tt.expectedErrorMsg {
					t.Errorf("error = %q, expected %q", err.Error(), tt.expectedErrorMsg)
				}
				if len(prResults) != 0 {
					t.Errorf("expected no results with an error, got %d", len(prResults))
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			numbers := make([]int, len(prResults))
			for i, result := range prResults {
				numbers[i] = result.pr.GetNumber()
			}
			if !reflect.DeepEqual(numbers, tt.expectedNumbers) {
				t.Errorf("PR numbers = %v, expected %v", numbers, tt.expectedNumbers)
			}
		})
	}
}

func TestListOpenPRsMapsNodeToPullRequest(t *testing.T) {
	transport := &recordingTransport{
		status: 200,
		responseBody: `{"data":{"rateLimit":{"cost":30,"remaining":4970,"limit":5000},` +
			`"r0":{"pullRequests":{"nodes":[` + pullRequestNodeJSON(1, "First") + `]}},` +
			`"r1":{"pullRequests":{"nodes":[]}}}}`,
	}
	testClient := &client{graphql: graphqlClient{transport: transport}}

	prResults, err := testClient.listOpenPRs(context.Background(), testRepositories)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prResults) != 1 {
		t.Fatalf("expected 1 result, got %d", len(prResults))
	}

	expected := PullRequest{
		Number:    1,
		Title:     "First",
		HTMLURL:   "https://github.com/owner-one/repo-one/pull/1",
		CreatedAt: time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC),
		State:     "open",
		Merged:    false,
		Draft:     false,
		Labels:    []string{"bug"},
		Author:    Collaborator{Login: "user1", Name: "User One"},
		HeadSHA:   "abc123",
	}
	if !reflect.DeepEqual(*prResults[0].pr, expected) {
		t.Errorf("pull request = %+v, expected %+v", *prResults[0].pr, expected)
	}
	if prResults[0].repository != testRepositories[0] {
		t.Errorf("repository = %+v, expected %+v", prResults[0].repository, testRepositories[0])
	}
}

const testEnrichFragmentName = "enrichedPr"

func TestBuildEnrichPRsQuery(t *testing.T) {
	references := []models.PullRequestRef{
		{Repository: testRepositories[0], Number: 111},
		{Repository: testRepositories[1], Number: 222},
	}

	query := buildPullRequestsQuery(references, enrichedPullRequestFragment)

	requiredFragments := []string{
		"query($owner0:String!,$name0:String!,$num0:Int!,$owner1:String!,$name1:String!,$num1:Int!)",
		"rateLimit { cost remaining limit }",
		"p0: repository(owner:$owner0,name:$name0){ pullRequest(number:$num0){ ..." + testEnrichFragmentName + " } }",
		"p1: repository(owner:$owner1,name:$name1){ pullRequest(number:$num1){ ..." + testEnrichFragmentName + " } }",
		"fragment " + testEnrichFragmentName + " on PullRequest {",
		"number\n  commits(last: 1){ nodes { commit { oid committedDate } } }",
		"reviews(first: 100){ nodes { state author { login __typename ... on User { name } } } }",
		"comments(first: 100){ nodes { createdAt body author { login __typename ... on User { name } } } }",
	}
	for _, fragment := range requiredFragments {
		if !strings.Contains(query.text, fragment) {
			t.Errorf("query text is missing %q, got:\n%s", fragment, query.text)
		}
	}

	forbiddenFragments := []string{
		"pullRequests(", "labels(", "owner-one", "owner-two", "repo-one", "repo-two", "111", "222",
	}
	for _, fragment := range forbiddenFragments {
		if strings.Contains(query.text, fragment) {
			t.Errorf("query text contains %q, got:\n%s", fragment, query.text)
		}
	}

	if aliasCount := strings.Count(query.text, ": repository(owner:"); aliasCount != len(references) {
		t.Errorf("query text has %d aliased repositories, expected %d", aliasCount, len(references))
	}
	spreadCount := strings.Count(query.text, "..."+testEnrichFragmentName+" }")
	if spreadCount != len(references) {
		t.Errorf("query text spreads the fragment %d times, expected %d", spreadCount, len(references))
	}

	expectedVariables := map[string]any{
		"owner0": "owner-one", "name0": "repo-one", "num0": 111,
		"owner1": "owner-two", "name1": "repo-two", "num1": 222,
	}
	if !reflect.DeepEqual(query.variables, expectedVariables) {
		t.Errorf("variables = %+v, expected %+v", query.variables, expectedVariables)
	}
	if !reflect.DeepEqual(query.aliases, []string{"p0", "p1"}) {
		t.Errorf("aliases = %v, expected [p0 p1]", query.aliases)
	}
	if !reflect.DeepEqual(query.repositoryByAlias["p1"], testRepositories[1]) {
		t.Errorf("repositoryByAlias[p1] = %+v, expected %+v", query.repositoryByAlias["p1"], testRepositories[1])
	}
}

const testFullFragmentName = "fullPr"

func TestBuildGetPRsQuery(t *testing.T) {
	references := []models.PullRequestRef{
		{Repository: testRepositories[0], Number: 111},
		{Repository: testRepositories[1], Number: 222},
	}

	query := buildPullRequestsQuery(references, fullPullRequestFragment)

	requiredFragments := []string{
		"query($owner0:String!,$name0:String!,$num0:Int!,$owner1:String!,$name1:String!,$num1:Int!)",
		"rateLimit { cost remaining limit }",
		"p0: repository(owner:$owner0,name:$name0){ pullRequest(number:$num0){ ..." +
			testFullFragmentName + " } }",
		"p1: repository(owner:$owner1,name:$name1){ pullRequest(number:$num1){ ..." +
			testFullFragmentName + " } }",
		"fragment " + testFullFragmentName + " on PullRequest {",
		"number title url isDraft createdAt updatedAt headRefOid state merged",
		"author { login __typename ... on User { name } }",
		"labels(first: 100){ nodes { name } }",
		"commits(last: 1){ nodes { commit { oid committedDate } } }",
		"reviews(first: 100){ nodes { state author { login __typename ... on User { name } } } }",
		"comments(first: 100){ nodes { createdAt body author { login __typename ... on User { name } } } }",
	}
	for _, fragment := range requiredFragments {
		if !strings.Contains(query.text, fragment) {
			t.Errorf("query text is missing %q, got:\n%s", fragment, query.text)
		}
	}

	forbiddenFragments := []string{
		"pullRequests(", "owner-one", "owner-two", "repo-one", "repo-two", "111", "222",
	}
	for _, fragment := range forbiddenFragments {
		if strings.Contains(query.text, fragment) {
			t.Errorf("query text contains %q, got:\n%s", fragment, query.text)
		}
	}

	if aliasCount := strings.Count(query.text, ": repository(owner:"); aliasCount != len(references) {
		t.Errorf("query text has %d aliased repositories, expected %d", aliasCount, len(references))
	}
	spreadCount := strings.Count(query.text, "..."+testFullFragmentName+" }")
	if spreadCount != len(references) {
		t.Errorf("query text spreads the fragment %d times, expected %d", spreadCount, len(references))
	}

	expectedVariables := map[string]any{
		"owner0": "owner-one", "name0": "repo-one", "num0": 111,
		"owner1": "owner-two", "name1": "repo-two", "num1": 222,
	}
	if !reflect.DeepEqual(query.variables, expectedVariables) {
		t.Errorf("variables = %+v, expected %+v", query.variables, expectedVariables)
	}
	if !reflect.DeepEqual(query.aliases, []string{"p0", "p1"}) {
		t.Errorf("aliases = %v, expected [p0 p1]", query.aliases)
	}
	if !reflect.DeepEqual(query.referenceByAlias["p1"], references[1]) {
		t.Errorf("referenceByAlias[p1] = %+v, expected %+v", query.referenceByAlias["p1"], references[1])
	}
}

func TestPullRequestFromNodeMapsStateAndMerged(t *testing.T) {
	tests := []struct {
		name           string
		nodeState      string
		nodeMerged     bool
		expectedState  string
		expectedMerged bool
	}{
		{name: "open PR", nodeState: "OPEN", expectedState: "open"},
		{name: "closed PR without merge", nodeState: "CLOSED", expectedState: "closed"},
		{
			name: "merged PR", nodeState: "MERGED", nodeMerged: true,
			expectedState: "closed", expectedMerged: true,
		},
		{name: "missing state renders as open", expectedState: "open"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pullRequest := pullRequestFromNode(
				pullRequestNode{State: tt.nodeState, Merged: tt.nodeMerged},
			)

			if pullRequest.GetState() != tt.expectedState {
				t.Errorf("GetState() = %q, expected %q", pullRequest.GetState(), tt.expectedState)
			}
			if pullRequest.GetMerged() != tt.expectedMerged {
				t.Errorf("GetMerged() = %t, expected %t", pullRequest.GetMerged(), tt.expectedMerged)
			}
		})
	}
}

// Unlike phase 2 under FindOpenPRs, an alias that carries no PR has nothing to fall back to.
func TestGetPRsByRefFailsOnNullPullRequestWithoutError(t *testing.T) {
	withoutRetryDelay(t)

	transport := &fakeEnrichTransport{
		fixtureByNumber: map[int]enrichFixture{2: {nullPullRequest: true}},
	}
	testClient := &client{graphql: graphqlClient{transport: transport}}

	references := []models.PullRequestRef{
		{Repository: testRepositories[0], Number: 1},
		{Repository: testRepositories[0], Number: 2},
	}
	prs, err := testClient.getPRsByRef(context.Background(), references)

	expectedMessage := "error fetching pull request owner-one/repo-one/2: no pull request returned"
	if err == nil {
		t.Fatalf("expected error %q, got %d PRs", expectedMessage, len(prs))
	}
	if !strings.Contains(err.Error(), expectedMessage) {
		t.Errorf("error = %q, expected it to contain %q", err.Error(), expectedMessage)
	}
}

type enrichFixture struct {
	reviews         []map[string]any
	comments        []map[string]any
	nullPullRequest bool
	errorType       string
	errorPath       []any // path suffix after the alias; the field it points at comes back null
	errorMessage    string
	requestStatus   int // non-zero: the whole request carrying this PR fails with this status
}

func (f enrichFixture) aliasResponse(number int) any {
	if f.errorType != "" && len(f.errorPath) == 0 {
		return nil
	}
	if f.nullPullRequest || len(f.errorPath) == 1 {
		return map[string]any{"pullRequest": nil}
	}
	pullRequest := map[string]any{
		"number":   number,
		"commits":  map[string]any{"nodes": []any{}},
		"reviews":  map[string]any{"nodes": f.reviews},
		"comments": map[string]any{"nodes": f.comments},
	}
	if len(f.errorPath) > 1 {
		pullRequest[f.errorPath[len(f.errorPath)-1].(string)] = nil
	}
	return map[string]any{"pullRequest": pullRequest}
}

type fakeEnrichTransport struct {
	fixtureByNumber  map[int]enrichFixture
	mutex            sync.Mutex
	requestedNumbers [][]int
}

func (t *fakeEnrichTransport) Post(_ context.Context, body []byte) (int, json.RawMessage, error) {
	var request graphqlRequest
	if err := json.Unmarshal(body, &request); err != nil {
		return 0, nil, err
	}
	numbers := postedPRNumbers(request.Variables)

	t.mutex.Lock()
	t.requestedNumbers = append(t.requestedNumbers, numbers)
	t.mutex.Unlock()

	data := map[string]any{"rateLimit": map[string]any{"cost": 1, "remaining": 4999, "limit": 5000}}
	responseErrors := []map[string]any{}

	for index, number := range numbers {
		fixture := t.fixtureByNumber[number]
		if fixture.requestStatus != 0 {
			return fixture.requestStatus, json.RawMessage(`{"message":"server error"}`), nil
		}
		alias := fmt.Sprintf("p%d", index)
		data[alias] = fixture.aliasResponse(number)
		if fixture.errorType != "" {
			responseErrors = append(responseErrors, map[string]any{
				"type":    fixture.errorType,
				"path":    append([]any{alias}, fixture.errorPath...),
				"message": fixture.errorMessage,
			})
		}
	}

	response := map[string]any{"data": data}
	if len(responseErrors) > 0 {
		response["errors"] = responseErrors
	}
	responseBody, err := json.Marshal(response)
	if err != nil {
		return 0, nil, err
	}
	return 200, responseBody, nil
}

func postedPRNumbers(variables map[string]any) []int {
	numbers := []int{}
	for index := 0; ; index++ {
		value, isSet := variables[fmt.Sprintf("num%d", index)]
		if !isSet {
			return numbers
		}
		numbers = append(numbers, int(value.(float64)))
	}
}

func authorNodeJSON(login string) map[string]any {
	return map[string]any{"login": login, "__typename": "User", "name": strings.ToUpper(login)}
}

func reviewNodeJSON(state, login string) map[string]any {
	return map[string]any{"state": state, "author": authorNodeJSON(login)}
}

func commentNodeJSON(login, body string, createdAt time.Time) map[string]any {
	return map[string]any{
		"createdAt": createdAt.Format(time.RFC3339),
		"body":      body,
		"author":    authorNodeJSON(login),
	}
}

func testPRResults(count int) []PRResult {
	results := make([]PRResult, count)
	for index := range results {
		results[index] = PRResult{
			pr: &PullRequest{
				Number: index + 1,
				Title:  fmt.Sprintf("PR %d", index+1),
				Author: Collaborator{Login: "pr-author"},
			},
			repository: testRepositories[0],
		}
	}
	return results
}

func reviewedAndCommentedFixtures(count int, overrides map[int]enrichFixture) map[int]enrichFixture {
	fixtures := map[int]enrichFixture{}
	for number := 1; number <= count; number++ {
		fixtures[number] = enrichFixture{
			reviews:  []map[string]any{reviewNodeJSON("APPROVED", "approver")},
			comments: []map[string]any{commentNodeJSON("commenter", "nice", time.Now())},
		}
	}
	maps.Copy(fixtures, overrides)
	return fixtures
}

func numbersUpTo(count int) []int {
	numbers := make([]int, count)
	for index := range numbers {
		numbers[index] = index + 1
	}
	return numbers
}

func assertPRNumbers(t *testing.T, prs []PR, expected []int) {
	t.Helper()
	numbers := make([]int, len(prs))
	for index, pr := range prs {
		numbers[index] = pr.GetNumber()
	}
	if !reflect.DeepEqual(numbers, expected) {
		t.Errorf("PR numbers = %v, expected %v", numbers, expected)
	}
}

func assertLogins(t *testing.T, label string, collaborators []Collaborator, expected []string) {
	t.Helper()
	logins := make([]string, len(collaborators))
	for index, collaborator := range collaborators {
		logins[index] = collaborator.Login
	}
	if len(logins) == 0 && len(expected) == 0 {
		return
	}
	if !reflect.DeepEqual(logins, expected) {
		t.Errorf("%s = %v, expected %v", label, logins, expected)
	}
}

func captureLogOutput(t *testing.T) *bytes.Buffer {
	t.Helper()
	var captured bytes.Buffer
	original := log.Writer()
	log.SetOutput(&captured)
	t.Cleanup(func() { log.SetOutput(original) })
	return &captured
}

func TestEnrichPRs(t *testing.T) {
	withoutRetryDelay(t)

	tests := []struct {
		name             string
		prCount          int
		fixtureByNumber  map[int]enrichFixture
		expectedRequests int
		expectedErrorMsg string
		assert           func(t *testing.T, prs []PR)
	}{
		{
			name:    "reviews and comments become approvers and commenters",
			prCount: 1,
			fixtureByNumber: map[int]enrichFixture{
				1: {
					reviews: []map[string]any{
						reviewNodeJSON("PENDING", "pending-reviewer"),
						reviewNodeJSON("APPROVED", "approver"),
						reviewNodeJSON("CHANGES_REQUESTED", "change-requester"),
					},
					comments: []map[string]any{commentNodeJSON("timeline-commenter", "looks good", time.Now())},
				},
			},
			expectedRequests: 1,
			assert: func(t *testing.T, prs []PR) {
				assertLogins(t, "approvers", prs[0].ApprovedByUsers, []string{"approver"})
				assertLogins(
					t, "commenters", prs[0].CommentedByUsers,
					[]string{"change-requester", "timeline-commenter"},
				)
			},
		},
		{
			name:             "25 PRs are enriched in one batch, in input order",
			prCount:          25,
			fixtureByNumber:  reviewedAndCommentedFixtures(25, nil),
			expectedRequests: 1,
			assert: func(t *testing.T, prs []PR) {
				assertPRNumbers(t, prs, numbersUpTo(25))
			},
		},
		{
			name:             "26 PRs are split into two batches, keeping input order",
			prCount:          26,
			fixtureByNumber:  reviewedAndCommentedFixtures(26, nil),
			expectedRequests: 2,
			assert: func(t *testing.T, prs []PR) {
				assertPRNumbers(t, prs, numbersUpTo(26))
			},
		},
		{
			name:    "a field error on one alias leaves the other 24 intact",
			prCount: 25,
			fixtureByNumber: reviewedAndCommentedFixtures(25, map[int]enrichFixture{
				3: {
					reviews:      []map[string]any{reviewNodeJSON("APPROVED", "approver")},
					errorType:    "RESOURCE_LIMITS_EXCEEDED",
					errorPath:    []any{"pullRequest", "comments"},
					errorMessage: "resource limit exceeded",
				},
			}),
			expectedRequests: 1,
			assert: func(t *testing.T, prs []PR) {
				assertPRNumbers(t, prs, numbersUpTo(25))
				assertLogins(t, "approvers of PR 3", prs[2].ApprovedByUsers, []string{"approver"})
				assertLogins(t, "commenters of PR 3", prs[2].CommentedByUsers, nil)
				assertLogins(t, "commenters of PR 4", prs[3].CommentedByUsers, []string{"commenter"})
			},
		},
		{
			name:    "a null pull request with no error leaves the other 24 intact",
			prCount: 25,
			fixtureByNumber: reviewedAndCommentedFixtures(25, map[int]enrichFixture{
				3: {nullPullRequest: true},
			}),
			expectedRequests: 1,
			assert: func(t *testing.T, prs []PR) {
				assertPRNumbers(t, prs, numbersUpTo(25))
				assertLogins(t, "approvers of PR 3", prs[2].ApprovedByUsers, nil)
				assertLogins(t, "commenters of PR 3", prs[2].CommentedByUsers, nil)
				assertLogins(t, "approvers of PR 4", prs[3].ApprovedByUsers, []string{"approver"})
			},
		},
		{
			name:    "a pull request error on one alias leaves the rest of the batch intact",
			prCount: 25,
			fixtureByNumber: reviewedAndCommentedFixtures(25, map[int]enrichFixture{
				3: {
					errorType:    "NOT_FOUND",
					errorPath:    []any{"pullRequest"},
					errorMessage: "Could not resolve to a PullRequest with the number 3.",
				},
			}),
			expectedRequests: 1,
			assert: func(t *testing.T, prs []PR) {
				assertPRNumbers(t, prs, numbersUpTo(25))
				assertLogins(t, "approvers of PR 3", prs[2].ApprovedByUsers, nil)
				assertLogins(t, "approvers of PR 4", prs[3].ApprovedByUsers, []string{"approver"})
			},
		},
		{
			name:    "a transport error on one batch fails the whole call",
			prCount: 26,
			fixtureByNumber: reviewedAndCommentedFixtures(26, map[int]enrichFixture{
				26: {requestStatus: 500},
			}),
			expectedErrorMsg: "error fetching reviews and comments: GraphQL request failed with status 500",
		},
		{
			name:    "a repository error fails the whole call, naming the repository",
			prCount: 3,
			fixtureByNumber: reviewedAndCommentedFixtures(3, map[int]enrichFixture{
				1: {errorType: "NOT_FOUND", errorMessage: "Could not resolve to a Repository."},
			}),
			expectedErrorMsg: "error fetching reviews and comments from owner-one/repo-one: " +
				"repository error on alias p0: NOT_FOUND Could not resolve to a Repository.",
		},
		{
			name:    "a snooze is parsed off the comments connection",
			prCount: 1,
			fixtureByNumber: map[int]enrichFixture{
				1: {comments: []map[string]any{
					commentNodeJSON("snoozer", "/snooze for 3 days", time.Now()),
				}},
			},
			expectedRequests: 1,
			assert: func(t *testing.T, prs []PR) {
				if prs[0].SnoozedUntil == nil {
					t.Fatal("SnoozedUntil = nil, expected a snooze three days out")
				}
				if !prs[0].SnoozedUntil.After(time.Now().Add(2 * 24 * time.Hour)) {
					t.Errorf("SnoozedUntil = %v, expected a snooze three days out", prs[0].SnoozedUntil)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeEnrichTransport{fixtureByNumber: tt.fixtureByNumber}
			testClient := &client{graphql: graphqlClient{transport: transport}}

			prs, err := testClient.enrichPRs(context.Background(), testPRResults(tt.prCount))

			if tt.expectedErrorMsg != "" {
				if err == nil {
					t.Fatalf("expected error %q, got none with %d PRs", tt.expectedErrorMsg, len(prs))
				}
				if !strings.Contains(err.Error(), tt.expectedErrorMsg) {
					t.Errorf("error = %q, expected it to contain %q", err.Error(), tt.expectedErrorMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(prs) != tt.prCount {
				t.Fatalf("got %d PRs, expected %d", len(prs), tt.prCount)
			}
			if len(transport.requestedNumbers) != tt.expectedRequests {
				t.Errorf(
					"made %d requests, expected %d: %v",
					len(transport.requestedNumbers), tt.expectedRequests, transport.requestedNumbers,
				)
			}
			tt.assert(t, prs)
		})
	}
}

func TestEnrichPRsFieldErrorOnCommentsLosesTheSnooze(t *testing.T) {
	withoutRetryDelay(t)
	logOutput := captureLogOutput(t)

	transport := &fakeEnrichTransport{fixtureByNumber: map[int]enrichFixture{
		1: {
			comments:     []map[string]any{commentNodeJSON("snoozer", "/snooze for 3 days", time.Now())},
			errorType:    "RESOURCE_LIMITS_EXCEEDED",
			errorPath:    []any{"pullRequest", "comments"},
			errorMessage: "resource limit exceeded",
		},
	}}
	testClient := &client{graphql: graphqlClient{transport: transport}}

	prs, err := testClient.enrichPRs(context.Background(), testPRResults(1))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("got %d PRs, expected 1", len(prs))
	}
	if prs[0].SnoozedUntil != nil {
		t.Errorf("SnoozedUntil = %v, expected nil - the comments connection failed", prs[0].SnoozedUntil)
	}
	if remaining := excludeSnoozedPRs(prs); len(remaining) != 1 {
		t.Errorf("the snoozed PR was excluded, expected it to reappear in the reminder")
	}

	for _, expected := range []string{
		"Unable to fetch reviews/comments for PR #1", "RESOURCE_LIMITS_EXCEEDED", "comments",
	} {
		if !strings.Contains(logOutput.String(), expected) {
			t.Errorf("log output is missing %q, got:\n%s", expected, logOutput.String())
		}
	}
}

const testMergedSinceDay = "2026-08-15"

var testMergedSince = time.Date(2026, 8, 15, 9, 30, 0, 0, time.UTC)

func TestBuildMergedPRsSearchQuery(t *testing.T) {
	query := buildMergedPRsSearchQuery(testRepositories, testMergedSince)

	requiredFragments := []string{
		"query($q0:String!,$q1:String!)",
		"rateLimit { cost remaining limit }",
		"s0: search(query:$q0, type: ISSUE, first: 100){ issueCount nodes { ... on PullRequest {",
		"s1: search(query:$q1, type: ISSUE, first: 100){ issueCount nodes { ... on PullRequest {",
		"number title url createdAt mergedAt",
		"author { login __typename ... on User { name } }",
		"labels(first: 100){ nodes { name } }",
	}
	for _, fragment := range requiredFragments {
		if !strings.Contains(query.text, fragment) {
			t.Errorf("query text is missing %q, got:\n%s", fragment, query.text)
		}
	}

	forbiddenFragments := []string{"orderBy", "isDraft", "updatedAt", "headRefOid", "fragment "}
	for _, fragment := range forbiddenFragments {
		if strings.Contains(query.text, fragment) {
			t.Errorf("query text contains %q, got:\n%s", fragment, query.text)
		}
	}

	expectedVariables := map[string]any{
		"q0": "repo:owner-one/repo-one is:pr is:merged merged:>=" + testMergedSinceDay,
		"q1": "repo:owner-two/repo-two is:pr is:merged merged:>=" + testMergedSinceDay,
	}
	if !reflect.DeepEqual(query.variables, expectedVariables) {
		t.Errorf("variables = %+v, expected %+v", query.variables, expectedVariables)
	}
	if !reflect.DeepEqual(query.aliases, []string{"s0", "s1"}) {
		t.Errorf("aliases = %v, expected [s0 s1]", query.aliases)
	}
	if !reflect.DeepEqual(query.repositoryByAlias["s1"], testRepositories[1]) {
		t.Errorf(
			"repositoryByAlias[s1] = %+v, expected %+v",
			query.repositoryByAlias["s1"], testRepositories[1],
		)
	}
}

// The cutoff qualifier is a day, so a merge time of any hour on that day names the same day.
func TestBuildMergedPRsSearchQueryCutsOffAtDayGranularity(t *testing.T) {
	query := buildMergedPRsSearchQuery(
		testRepositories[:1], time.Date(2026, 8, 15, 23, 59, 59, 0, time.UTC),
	)

	expectedQualifier := "merged:>=" + testMergedSinceDay
	if !strings.HasSuffix(query.variables["q0"].(string), expectedQualifier) {
		t.Errorf("q0 = %q, expected it to end with %q", query.variables["q0"], expectedQualifier)
	}
}

func mergedPullRequestNodeJSON(number int, title, mergedAt string) string {
	return fmt.Sprintf(
		`{"number":%d,"title":%q,"url":"https://github.com/owner-one/repo-one/pull/%d",`+
			`"createdAt":"2026-05-01T12:00:00Z","mergedAt":%q,`+
			`"author":{"login":"user1","__typename":"User","name":"User One"},`+
			`"labels":{"nodes":[{"name":"bug"}]}}`,
		number, title, number, mergedAt,
	)
}

func searchResponseJSON(issueCountByAlias map[string]int, nodesByAlias map[string][]string) string {
	aliasResults := []string{`"rateLimit":{"cost":2,"remaining":4998,"limit":5000}`}
	for _, alias := range []string{"s0", "s1"} {
		nodes, isSet := nodesByAlias[alias]
		if !isSet {
			continue
		}
		aliasResults = append(aliasResults, fmt.Sprintf(
			`%q:{"issueCount":%d,"nodes":[%s]}`,
			alias, issueCountByAlias[alias], strings.Join(nodes, ","),
		))
	}
	return `{"data":{` + strings.Join(aliasResults, ",") + `}}`
}

func TestSearchMergedPRsMapsNodesPerAlias(t *testing.T) {
	transport := &recordingTransport{
		status: 200,
		responseBody: searchResponseJSON(
			map[string]int{"s0": 1, "s1": 1},
			map[string][]string{
				"s0": {mergedPullRequestNodeJSON(1, "First", "2026-08-20T12:00:00Z")},
				"s1": {mergedPullRequestNodeJSON(2, "Second", "2026-08-21T12:00:00Z")},
			},
		),
	}
	testClient := &client{graphql: graphqlClient{transport: transport}}

	prResults, err := testClient.searchMergedPRs(
		context.Background(), testRepositories, testMergedSince,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prResults) != 2 {
		t.Fatalf("expected 2 results, got %d", len(prResults))
	}

	mergedAt := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	expected := PullRequest{
		Number:    1,
		Title:     "First",
		HTMLURL:   "https://github.com/owner-one/repo-one/pull/1",
		CreatedAt: time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
		State:     "closed",
		Merged:    true,
		MergedAt:  &mergedAt,
		Labels:    []string{"bug"},
		Author:    Collaborator{Login: "user1", Name: "User One"},
	}
	if !reflect.DeepEqual(*prResults[0].pr, expected) {
		t.Errorf("pull request = %+v, expected %+v", *prResults[0].pr, expected)
	}
	if prResults[0].repository != testRepositories[0] {
		t.Errorf("repository = %+v, expected %+v", prResults[0].repository, testRepositories[0])
	}
	if prResults[1].repository != testRepositories[1] {
		t.Errorf("repository = %+v, expected %+v", prResults[1].repository, testRepositories[1])
	}
}

func TestSearchMergedPRsLogsTruncatedRepository(t *testing.T) {
	captured := captureLogOutput(t)
	transport := &recordingTransport{
		status: 200,
		responseBody: searchResponseJSON(
			map[string]int{"s0": 101, "s1": 100},
			map[string][]string{
				"s0": {mergedPullRequestNodeJSON(1, "First", "2026-08-20T12:00:00Z")},
				"s1": {mergedPullRequestNodeJSON(2, "Second", "2026-08-20T12:00:00Z")},
			},
		),
	}
	testClient := &client{graphql: graphqlClient{transport: transport}}

	if _, err := testClient.searchMergedPRs(
		context.Background(), testRepositories, testMergedSince,
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	logged := captured.String()
	if !strings.Contains(logged, "owner-one/repo-one") || !strings.Contains(logged, "101") {
		t.Errorf("expected a truncation log line naming owner-one/repo-one and 101, got:\n%s", logged)
	}
	if strings.Contains(logged, "owner-two/repo-two") {
		t.Errorf("expected no truncation log line for the repository at the page size, got:\n%s", logged)
	}
}

func TestSearchMergedPRsFailsOnAnyError(t *testing.T) {
	withoutRetryDelay(t)

	tests := []struct {
		name             string
		status           int
		responseBody     string
		expectedErrorMsg string
	}{
		{
			name:   "whole-query error",
			status: 200,
			responseBody: `{"data":null,"errors":[{"type":"RATE_LIMITED",` +
				`"message":"API rate limit exceeded"}]}`,
			expectedErrorMsg: "error fetching merged pull requests: " +
				"query error: RATE_LIMITED API rate limit exceeded",
		},
		{
			name:   "error scoped to one search alias",
			status: 200,
			responseBody: `{"data":null,"errors":[{"type":"EXCESSIVE_PAGINATION","path":["s0"],` +
				`"message":"Requesting 200 records exceeds the first 100 limit"}]}`,
			expectedErrorMsg: "error fetching merged pull requests: repository error on alias s0: " +
				"EXCESSIVE_PAGINATION Requesting 200 records exceeds the first 100 limit",
		},
		{
			name:             "alias missing from the data",
			status:           200,
			responseBody:     searchResponseJSON(map[string]int{"s0": 0}, map[string][]string{"s0": {}}),
			expectedErrorMsg: "error fetching merged pull requests: no data returned for owner-two/repo-two",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &recordingTransport{status: tt.status, responseBody: tt.responseBody}
			testClient := &client{graphql: graphqlClient{transport: transport}}

			prResults, err := testClient.searchMergedPRs(
				context.Background(), testRepositories, testMergedSince,
			)

			if err == nil {
				t.Fatalf("expected error %q, got none with %d results", tt.expectedErrorMsg, len(prResults))
			}
			if err.Error() != tt.expectedErrorMsg {
				t.Errorf("error = %q, expected %q", err.Error(), tt.expectedErrorMsg)
			}
		})
	}
}

func mergedNodesSinceJSON(mergedAts ...string) []string {
	nodes := make([]string, len(mergedAts))
	for index, mergedAt := range mergedAts {
		nodes[index] = mergedPullRequestNodeJSON(index+1, fmt.Sprintf("PR %d", index+1), mergedAt)
	}
	return nodes
}

// The day-granularity qualifier returns up to one extra day of merges, so the exact cut is here.
func TestFindRecentlyMergedPRsCutsTheWindowClientSide(t *testing.T) {
	transport := &recordingTransport{
		status: 200,
		responseBody: searchResponseJSON(
			map[string]int{"s0": 3},
			map[string][]string{"s0": {
				mergedPullRequestNodeJSON(1, "Merged inside the window", "2026-08-15T10:00:00Z"),
				mergedPullRequestNodeJSON(2, "Merged before the window", "2026-08-15T08:00:00Z"),
				mergedPullRequestNodeJSON(4, "Merged at the window start", "2026-08-15T09:30:00Z"),
				strings.Replace(
					mergedPullRequestNodeJSON(3, "Never merged", ""), `"mergedAt":""`, `"mergedAt":null`, 1,
				),
			}},
		),
	}
	testClient := &client{graphql: graphqlClient{transport: transport}}

	prs, err := testClient.FindRecentlyMergedPRs(
		context.Background(), testRepositories[:1], noTestFilters, testMergedSince,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if numbers := prNumbersOf(prs); !reflect.DeepEqual(numbers, []int{1, 4}) {
		t.Errorf("PR numbers = %v, expected [1 4]", numbers)
	}
}

func TestFindRecentlyMergedPRsKeepsTheNewestUpToTheCap(t *testing.T) {
	if MaxMergedPRsToFetch != 15 {
		t.Fatalf("MaxMergedPRsToFetch = %d, expected 15", MaxMergedPRsToFetch)
	}

	mergedAts := make([]string, MaxMergedPRsToFetch+3)
	for index := range mergedAts {
		mergedAts[index] = fmt.Sprintf("2026-08-16T%02d:00:00Z", index)
	}
	transport := &recordingTransport{
		status: 200,
		responseBody: searchResponseJSON(
			map[string]int{"s0": len(mergedAts)},
			map[string][]string{"s0": mergedNodesSinceJSON(mergedAts...)},
		),
	}
	testClient := &client{graphql: graphqlClient{transport: transport}}

	prs, err := testClient.FindRecentlyMergedPRs(
		context.Background(), testRepositories[:1], noTestFilters, testMergedSince,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedNumbers := make([]int, MaxMergedPRsToFetch)
	for index := range expectedNumbers {
		expectedNumbers[index] = len(mergedAts) - index
	}
	if numbers := prNumbersOf(prs); !reflect.DeepEqual(numbers, expectedNumbers) {
		t.Errorf("PR numbers = %v, expected %v", numbers, expectedNumbers)
	}
}

// Reviewers and the snooze are never fetched for a merged PR.
func TestFindRecentlyMergedPRsFetchesNoReviewers(t *testing.T) {
	transport := &recordingTransport{
		status: 200,
		responseBody: searchResponseJSON(
			map[string]int{"s0": 1},
			map[string][]string{"s0": mergedNodesSinceJSON("2026-08-20T12:00:00Z")},
		),
	}
	testClient := &client{graphql: graphqlClient{transport: transport}}

	prs, err := testClient.FindRecentlyMergedPRs(
		context.Background(), testRepositories[:1], noTestFilters, testMergedSince,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if transport.calls != 1 {
		t.Errorf("GraphQL requests = %d, expected 1", transport.calls)
	}
	if len(prs) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(prs))
	}
	if prs[0].ApprovedByUsers != nil || prs[0].CommentedByUsers != nil || prs[0].SnoozedUntil != nil {
		t.Errorf("expected no reviewers and no snooze on a merged PR, got %+v", prs[0])
	}
}

func prNumbersOf(prs []PR) []int {
	numbers := make([]int, len(prs))
	for index, pr := range prs {
		numbers[index] = pr.GetNumber()
	}
	return numbers
}

func noTestFilters(models.Repository) config.Filters {
	return config.Filters{}
}

func TestFindRecentlyMergedPRsAppliesFilters(t *testing.T) {
	excludedNode := fmt.Sprintf(
		`{"number":1,"title":"Bump the SDK","url":"https://github.com/owner-one/repo-one/pull/1",`+
			`"createdAt":"2026-05-01T12:00:00Z","mergedAt":"2026-08-20T12:00:00Z",`+
			`"author":{"login":"dependabot","__typename":"User","name":"Dependabot"},`+
			`"labels":{"nodes":[{"name":%q}]}}`, "dependencies",
	)
	keptNode := mergedPullRequestNodeJSON(2, "Add pagination", "2026-08-20T13:00:00Z")

	tests := []struct {
		name    string
		filters config.Filters
	}{
		{name: "ignored label", filters: config.Filters{IgnoredLabels: []string{"dependencies"}}},
		{name: "label allow list", filters: config.Filters{Labels: []string{"bug"}}},
		{name: "ignored author", filters: config.Filters{IgnoredAuthors: []string{"dependabot"}}},
		{name: "author allow list", filters: config.Filters{Authors: []string{"user1"}}},
		{name: "ignored title term", filters: config.Filters{IgnoredTerms: []string{"Bump"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &recordingTransport{
				status: 200,
				responseBody: searchResponseJSON(
					map[string]int{"s0": 2},
					map[string][]string{"s0": {excludedNode, keptNode}},
				),
			}
			testClient := &client{graphql: graphqlClient{transport: transport}}

			prs, err := testClient.FindRecentlyMergedPRs(
				context.Background(),
				testRepositories[:1],
				func(models.Repository) config.Filters { return tt.filters },
				testMergedSince,
			)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if numbers := prNumbersOf(prs); !reflect.DeepEqual(numbers, []int{2}) {
				t.Errorf("PR numbers = %v, expected [2]", numbers)
			}
		})
	}
}
