package githubclient

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/internal/models"
)

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

	forbiddenFragments := []string{
		"orderBy", "isDraft", "updatedAt", "commits", "headRefOid", "fragment ",
	}
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
	if MaxMergedPRsToFetch != 6 {
		t.Fatalf("MaxMergedPRsToFetch = %d, expected 6", MaxMergedPRsToFetch)
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
