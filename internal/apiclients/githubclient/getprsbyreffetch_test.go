package githubclient

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/hellej/pr-slack-reminder-action/internal/models"
)

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
		"number title url isDraft createdAt updatedAt state merged",
		"author { login __typename ... on User { name } }",
		"labels(first: 100){ nodes { name } }",
		"reviews(first: 100){ nodes { state author { login __typename ... on User { name } } } }",
		"comments(first: 100){ nodes { createdAt body author { login __typename ... on User { name } } } }",
	}
	for _, fragment := range requiredFragments {
		if !strings.Contains(query.text, fragment) {
			t.Errorf("query text is missing %q, got:\n%s", fragment, query.text)
		}
	}

	forbiddenFragments := []string{
		"pullRequests(", "commits", "headRefOid",
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
