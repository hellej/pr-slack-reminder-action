package githubclient

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/models"
)

var testRepositories = []models.Repository{
	models.NewRepository("owner-one", "repo-one"),
	models.NewRepository("owner-two", "repo-two"),
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
