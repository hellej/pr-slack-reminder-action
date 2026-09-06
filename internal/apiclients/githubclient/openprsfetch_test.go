package githubclient

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/models"
)

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
		"number title url isDraft createdAt updatedAt",
		"author { login __typename ... on User { name } }",
		"labels(first: 100) { nodes { name } }",
	}
	for _, fragment := range requiredFragments {
		if !strings.Contains(query.text, fragment) {
			t.Errorf("query text is missing %q, got:\n%s", fragment, query.text)
		}
	}

	// Neither is read any more. See docs/third-party-facts.md on the commits permission.
	forbiddenFragments := []string{"commits", "headRefOid"}
	for _, fragment := range forbiddenFragments {
		if strings.Contains(query.text, fragment) {
			t.Errorf("query text contains %q, got:\n%s", fragment, query.text)
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
			`"createdAt":"2026-05-01T12:00:00Z","updatedAt":"2026-05-02T12:00:00Z",`+
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
		"number\n  reviews(first: 100){ nodes { state author { login __typename ... on User { name } } } }",
		"comments(first: 100){ nodes { createdAt body author { login __typename ... on User { name } } } }",
	}
	for _, fragment := range requiredFragments {
		if !strings.Contains(query.text, fragment) {
			t.Errorf("query text is missing %q, got:\n%s", fragment, query.text)
		}
	}

	forbiddenFragments := []string{
		"pullRequests(", "labels(", "commits", "headRefOid",
		"owner-one", "owner-two", "repo-one", "repo-two", "111", "222",
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

			prs, err := testClient.enrichPRsWithReviewInfo(context.Background(), testPRResults(tt.prCount))

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

	prs, err := testClient.enrichPRsWithReviewInfo(context.Background(), testPRResults(1))
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
