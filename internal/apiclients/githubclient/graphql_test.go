package githubclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

type recordingTransport struct {
	status        int
	responseBody  string
	err           error
	responses     []recordedResponse
	calls         int
	lastPostedRaw []byte
}

type recordedResponse struct {
	status       int
	responseBody string
	err          error
}

func (t *recordingTransport) Post(ctx context.Context, body []byte) (int, json.RawMessage, error) {
	t.lastPostedRaw = body
	response := recordedResponse{status: t.status, responseBody: t.responseBody, err: t.err}
	if len(t.responses) > 0 {
		response = t.responses[min(t.calls, len(t.responses)-1)]
	}
	t.calls++
	return response.status, json.RawMessage(response.responseBody), response.err
}

type testAliasData struct {
	Number int `json:"number"`
}

type testResponseData struct {
	R0 *testAliasData `json:"r0"`
	P0 *testAliasData `json:"p0"`
}

func withoutRetryDelay(t *testing.T) {
	t.Helper()
	original := retryDelay
	retryDelay = 0
	t.Cleanup(func() { retryDelay = original })
}

func TestGraphQLDo(t *testing.T) {
	withoutRetryDelay(t)

	tests := []struct {
		name                string
		status              int
		responseBody        string
		aliases             []string
		expectedAttempts    int
		expectedErrorClass  string
		expectedErrorAlias  string
		expectedErrorType   string
		expectedErrorCode   string
		expectedFieldErrors []fieldError
		expectedData        testResponseData
	}{
		{
			name:             "data is decoded when there are no errors",
			status:           200,
			responseBody:     `{"data":{"r0":{"number":7}}}`,
			aliases:          []string{"r0"},
			expectedAttempts: 1,
			expectedData:     testResponseData{R0: &testAliasData{Number: 7}},
		},
		{
			name:   "error on an alias is a repository error",
			status: 200,
			responseBody: `{"data":{"r0":null},"errors":[{"type":"NOT_FOUND","path":["r0"],` +
				`"message":"Could not resolve to a Repository with the name 'owner/repo'."}]}`,
			aliases:            []string{"r0"},
			expectedAttempts:   1,
			expectedErrorClass: "repository",
			expectedErrorAlias: "r0",
			expectedErrorType:  "NOT_FOUND",
		},
		{
			name:   "error on alias.pullRequest is a pull request error",
			status: 200,
			responseBody: `{"data":{"p0":{"pullRequest":null}},"errors":[{"type":"NOT_FOUND",` +
				`"path":["p0","pullRequest"],"message":"Could not resolve to a PullRequest with the number of 5."}]}`,
			aliases:            []string{"p0"},
			expectedAttempts:   1,
			expectedErrorClass: "pullRequest",
			expectedErrorAlias: "p0",
			expectedErrorType:  "NOT_FOUND",
			expectedData:       testResponseData{P0: &testAliasData{}},
		},
		{
			name:   "nested error on an alias is a field error returned with the data",
			status: 200,
			responseBody: `{"data":{"p0":{"number":5}},"errors":[{"type":"RESOURCE_LIMITS_EXCEEDED",` +
				`"path":["p0","pullRequest","comments"],"message":"resource limit exceeded"}]}`,
			aliases:          []string{"p0"},
			expectedAttempts: 1,
			expectedFieldErrors: []fieldError{{
				alias:     "p0",
				path:      []any{"p0", "pullRequest", "comments"},
				errorType: "RESOURCE_LIMITS_EXCEEDED",
				message:   "resource limit exceeded",
			}},
			expectedData: testResponseData{P0: &testAliasData{Number: 5}},
		},
		{
			name:   "several nested errors are all returned as field errors",
			status: 200,
			responseBody: `{"data":{"p0":{"number":5}},"errors":[` +
				`{"type":"RESOURCE_LIMITS_EXCEEDED","path":["p0","pullRequest","comments"],"message":"comments limit"},` +
				`{"type":"RESOURCE_LIMITS_EXCEEDED","path":["p0","pullRequest","reviews"],"message":"reviews limit"}]}`,
			aliases:          []string{"p0"},
			expectedAttempts: 1,
			expectedFieldErrors: []fieldError{
				{
					alias:     "p0",
					path:      []any{"p0", "pullRequest", "comments"},
					errorType: "RESOURCE_LIMITS_EXCEEDED",
					message:   "comments limit",
				},
				{
					alias:     "p0",
					path:      []any{"p0", "pullRequest", "reviews"},
					errorType: "RESOURCE_LIMITS_EXCEEDED",
					message:   "reviews limit",
				},
			},
			expectedData: testResponseData{P0: &testAliasData{Number: 5}},
		},
		{
			name:   "hard error keeps the data of the aliases that resolved",
			status: 200,
			responseBody: `{"data":{"p0":{"number":42}},"errors":[{"type":"NOT_FOUND",` +
				`"path":["p3","pullRequest"],"message":"Could not resolve to a PullRequest with the number of 9."}]}`,
			aliases:            []string{"p0", "p3"},
			expectedAttempts:   1,
			expectedErrorClass: "pullRequest",
			expectedErrorAlias: "p3",
			expectedErrorType:  "NOT_FOUND",
			expectedData:       testResponseData{P0: &testAliasData{Number: 42}},
		},
		{
			name:   "hard error wins over the field errors it arrives with",
			status: 200,
			responseBody: `{"data":{"p0":{"number":42}},"errors":[` +
				`{"type":"RESOURCE_LIMITS_EXCEEDED","path":["p0","pullRequest","comments"],"message":"comments limit"},` +
				`{"type":"FORBIDDEN","path":["p3"],"message":"no access"}]}`,
			aliases:            []string{"p0", "p3"},
			expectedAttempts:   1,
			expectedErrorClass: "repository",
			expectedErrorAlias: "p3",
			expectedErrorType:  "FORBIDDEN",
			expectedFieldErrors: []fieldError{{
				alias:     "p0",
				path:      []any{"p0", "pullRequest", "comments"},
				errorType: "RESOURCE_LIMITS_EXCEEDED",
				message:   "comments limit",
			}},
			expectedData: testResponseData{P0: &testAliasData{Number: 42}},
		},
		{
			name:               "error without a path is a query error",
			status:             200,
			responseBody:       `{"data":null,"errors":[{"type":"RATE_LIMITED","message":"API rate limit exceeded"}]}`,
			aliases:            []string{"r0"},
			expectedAttempts:   1,
			expectedErrorClass: "query",
			expectedErrorType:  "RATE_LIMITED",
		},
		{
			name:   "error rooted at query is a query error",
			status: 200,
			responseBody: `{"errors":[{"path":["query","repository","notAField"],` +
				`"extensions":{"code":"undefinedField"},` +
				`"message":"Field 'notAField' doesn't exist on type 'Repository'"}]}`,
			aliases:            []string{"r0"},
			expectedAttempts:   1,
			expectedErrorClass: "query",
			expectedErrorCode:  "undefinedField",
		},
		{
			name:   "error rooted at an unknown alias is a query error",
			status: 200,
			responseBody: `{"data":{"r0":{"number":7}},"errors":[{"type":"NOT_FOUND",` +
				`"path":["r9","pullRequests","nodes"],"message":"unknown alias"}]}`,
			aliases:            []string{"r0"},
			expectedAttempts:   1,
			expectedErrorClass: "query",
			expectedErrorType:  "NOT_FOUND",
			expectedData:       testResponseData{R0: &testAliasData{Number: 7}},
		},
		{
			name:               "error with a non-string path root is a query error",
			status:             200,
			responseBody:       `{"data":{"r0":{"number":7}},"errors":[{"type":"NOT_FOUND","path":[0,"nodes"],"message":"odd path"}]}`,
			aliases:            []string{"r0"},
			expectedAttempts:   1,
			expectedErrorClass: "query",
			expectedErrorType:  "NOT_FOUND",
			expectedData:       testResponseData{R0: &testAliasData{Number: 7}},
		},
		{
			name:               "null data without errors is a query error",
			status:             200,
			responseBody:       `{"data":null}`,
			aliases:            []string{"r0"},
			expectedAttempts:   1,
			expectedErrorClass: "query",
		},
		{
			name:               "missing data without errors is a query error",
			status:             200,
			responseBody:       `{}`,
			aliases:            []string{"r0"},
			expectedAttempts:   1,
			expectedErrorClass: "query",
		},
		{
			name:               "server error is retried once and then fails",
			status:             502,
			responseBody:       `{"message":"Server Error"}`,
			aliases:            []string{"r0"},
			expectedAttempts:   2,
			expectedErrorClass: "transport",
		},
		{
			name:               "rate limit status is retried once and then fails",
			status:             429,
			responseBody:       `{"message":"Too Many Requests"}`,
			aliases:            []string{"r0"},
			expectedAttempts:   2,
			expectedErrorClass: "transport",
		},
		{
			name:               "unauthorized fails without a retry",
			status:             401,
			responseBody:       `{"message":"Bad credentials"}`,
			aliases:            []string{"r0"},
			expectedAttempts:   1,
			expectedErrorClass: "transport",
		},
		{
			name:               "forbidden fails without a retry",
			status:             403,
			responseBody:       `{"message":"Forbidden"}`,
			aliases:            []string{"r0"},
			expectedAttempts:   1,
			expectedErrorClass: "transport",
		},
		{
			name:               "unparseable body is retried once and then fails",
			status:             200,
			responseBody:       `<html>not json</html>`,
			aliases:            []string{"r0"},
			expectedAttempts:   2,
			expectedErrorClass: "transport",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &recordingTransport{status: tt.status, responseBody: tt.responseBody}
			client := graphqlClient{transport: transport}

			var data testResponseData
			fieldErrors, err := client.Do(context.Background(), "query{}", nil, tt.aliases, &data)

			assertErrorClass(t, err, tt.expectedErrorClass, tt.expectedErrorAlias, tt.expectedErrorType, tt.expectedErrorCode)
			assertFieldErrors(t, fieldErrors, tt.expectedFieldErrors)

			if transport.calls != tt.expectedAttempts {
				t.Errorf("expected %d attempts, got %d", tt.expectedAttempts, transport.calls)
			}
			assertAliasData(t, "r0", data.R0, tt.expectedData.R0)
			assertAliasData(t, "p0", data.P0, tt.expectedData.P0)
		})
	}
}

func assertErrorClass(t *testing.T, err error, class, alias, errorType, code string) {
	t.Helper()

	switch class {
	case "":
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	case "repository":
		var classified repositoryError
		if !errors.As(err, &classified) {
			t.Fatalf("expected repositoryError, got %#v", err)
		}
		assertEqualStrings(t, "alias", classified.alias, alias)
		assertEqualStrings(t, "error type", classified.errorType, errorType)
	case "pullRequest":
		var classified pullRequestError
		if !errors.As(err, &classified) {
			t.Fatalf("expected pullRequestError, got %#v", err)
		}
		assertEqualStrings(t, "alias", classified.alias, alias)
		assertEqualStrings(t, "error type", classified.errorType, errorType)
	case "query":
		var classified queryError
		if !errors.As(err, &classified) {
			t.Fatalf("expected queryError, got %#v", err)
		}
		assertEqualStrings(t, "error type", classified.errorType, errorType)
		assertEqualStrings(t, "error code", classified.code, code)
	case "transport":
		var classified transportError
		if !errors.As(err, &classified) {
			t.Fatalf("expected transportError, got %#v", err)
		}
	default:
		t.Fatalf("unknown expected error class %q", class)
	}
}

func assertFieldErrors(t *testing.T, actual, expected []fieldError) {
	t.Helper()

	if len(actual) != len(expected) {
		t.Fatalf("expected %d field errors, got %d (%v)", len(expected), len(actual), actual)
	}
	for i, expectedFieldError := range expected {
		assertEqualStrings(t, "field error alias", actual[i].alias, expectedFieldError.alias)
		assertEqualStrings(t, "field error type", actual[i].errorType, expectedFieldError.errorType)
		assertEqualStrings(t, "field error message", actual[i].message, expectedFieldError.message)
		assertEqualStrings(t, "field error path", formatPath(actual[i].path), formatPath(expectedFieldError.path))
	}
}

func assertAliasData(t *testing.T, alias string, actual, expected *testAliasData) {
	t.Helper()

	if expected == nil {
		if actual != nil {
			t.Errorf("expected no data for alias %s, got %+v", alias, actual)
		}
		return
	}
	if actual == nil {
		t.Fatalf("expected data for alias %s, got none", alias)
	}
	if actual.Number != expected.Number {
		t.Errorf("expected number %d for alias %s, got %d", expected.Number, alias, actual.Number)
	}
}

func assertEqualStrings(t *testing.T, name, actual, expected string) {
	t.Helper()

	if actual != expected {
		t.Errorf("expected %s %q, got %q", name, expected, actual)
	}
}

func TestGraphQLDoRetrySucceeds(t *testing.T) {
	withoutRetryDelay(t)

	tests := []struct {
		name            string
		firstAttempt    recordedResponse
		expectedRetried bool
	}{
		{
			name:            "server error",
			firstAttempt:    recordedResponse{status: 502, responseBody: `{"message":"Server Error"}`},
			expectedRetried: true,
		},
		{
			name:            "network error",
			firstAttempt:    recordedResponse{err: errors.New("connection reset")},
			expectedRetried: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &recordingTransport{responses: []recordedResponse{
				tt.firstAttempt,
				{status: 200, responseBody: `{"data":{"r0":{"number":3}}}`},
			}}
			client := graphqlClient{transport: transport}

			var data testResponseData
			fieldErrors, err := client.Do(context.Background(), "query{}", nil, []string{"r0"}, &data)

			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if len(fieldErrors) != 0 {
				t.Errorf("expected no field errors, got %v", fieldErrors)
			}
			if transport.calls != 2 {
				t.Errorf("expected 2 attempts, got %d", transport.calls)
			}
			assertAliasData(t, "r0", data.R0, &testAliasData{Number: 3})
		})
	}
}

func TestGraphQLDoReturnsMostSevereError(t *testing.T) {
	withoutRetryDelay(t)

	pullRequestNotFound := `{"type":"NOT_FOUND","path":["p3","pullRequest"],"message":"no pull request"}`
	repositoryForbidden := `{"type":"FORBIDDEN","path":["p7"],"message":"no access"}`
	rateLimited := `{"type":"RATE_LIMITED","message":"API rate limit exceeded"}`

	tests := []struct {
		name               string
		responseErrors     []string
		expectedErrorClass string
		expectedErrorAlias string
		expectedErrorType  string
	}{
		{
			name:               "repository error over pull request error",
			responseErrors:     []string{pullRequestNotFound, repositoryForbidden},
			expectedErrorClass: "repository",
			expectedErrorAlias: "p7",
			expectedErrorType:  "FORBIDDEN",
		},
		{
			name:               "repository error over pull request error, reversed",
			responseErrors:     []string{repositoryForbidden, pullRequestNotFound},
			expectedErrorClass: "repository",
			expectedErrorAlias: "p7",
			expectedErrorType:  "FORBIDDEN",
		},
		{
			name:               "query error over repository error",
			responseErrors:     []string{repositoryForbidden, rateLimited},
			expectedErrorClass: "query",
			expectedErrorType:  "RATE_LIMITED",
		},
		{
			name:               "query error over repository error, reversed",
			responseErrors:     []string{rateLimited, repositoryForbidden},
			expectedErrorClass: "query",
			expectedErrorType:  "RATE_LIMITED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			responseBody := fmt.Sprintf(
				`{"data":{"p0":{"number":42}},"errors":[%s]}`, strings.Join(tt.responseErrors, ","),
			)
			transport := &recordingTransport{status: 200, responseBody: responseBody}
			client := graphqlClient{transport: transport}

			var data testResponseData
			_, err := client.Do(context.Background(), "query{}", nil, []string{"p0", "p3", "p7"}, &data)

			assertErrorClass(t, err, tt.expectedErrorClass, tt.expectedErrorAlias, tt.expectedErrorType, "")
		})
	}
}

func TestGraphQLDoStopsRetryingOnCancelledContext(t *testing.T) {
	original := retryDelay
	retryDelay = 5 * time.Second
	t.Cleanup(func() { retryDelay = original })

	ctx, cancel := context.WithCancel(context.Background())
	transport := &cancellingTransport{cancel: cancel}
	client := graphqlClient{transport: transport}

	_, err := client.Do(ctx, "query{}", nil, []string{"r0"}, &testResponseData{})

	assertErrorClass(t, err, "transport", "", "", "")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected a cancelled context error, got %v", err)
	}
	if transport.calls != 1 {
		t.Errorf("expected 1 attempt, got %d", transport.calls)
	}
}

type cancellingTransport struct {
	cancel context.CancelFunc
	calls  int
}

func (t *cancellingTransport) Post(ctx context.Context, body []byte) (int, json.RawMessage, error) {
	t.calls++
	t.cancel()
	return 502, json.RawMessage(`{"message":"Server Error"}`), nil
}

func TestGraphQLDoPostsQueryAndVariables(t *testing.T) {
	withoutRetryDelay(t)

	transport := &recordingTransport{status: 200, responseBody: `{"data":{}}`}
	client := graphqlClient{transport: transport}

	query := "query($owner0:String!){ r0: repository(owner:$owner0) { name } }"
	variables := map[string]any{"owner0": "testowner"}
	if _, err := client.Do(context.Background(), query, variables, []string{"r0"}, &testResponseData{}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var posted struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	if err := json.Unmarshal(transport.lastPostedRaw, &posted); err != nil {
		t.Fatalf("posted body is not JSON: %v", err)
	}
	assertEqualStrings(t, "query", posted.Query, query)
	if posted.Variables["owner0"] != "testowner" {
		t.Errorf("expected variable owner0 to be testowner, got %v", posted.Variables["owner0"])
	}
}
