package githubclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"testing"

	"github.com/hellej/pr-slack-reminder-action/internal/models"
)

var testRepositories = []models.Repository{
	{Owner: "owner-one", Name: "repo-one"},
	{Owner: "owner-two", Name: "repo-two"},
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

func captureLogOutput(t *testing.T) *bytes.Buffer {
	t.Helper()
	var captured bytes.Buffer
	original := log.Writer()
	log.SetOutput(&captured)
	t.Cleanup(func() { log.SetOutput(original) })
	return &captured
}
