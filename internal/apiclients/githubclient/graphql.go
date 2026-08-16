package githubclient

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"time"
)

const graphqlEndpoint = "https://api.github.com/graphql"

const graphqlUserAgent = "pr-slack-reminder-action"

const graphqlMaxAttempts = 2

// Not a const so that in-package tests can zero it.
var retryDelay = 1 * time.Second

type graphqlTransport interface {
	Post(ctx context.Context, body []byte) (status int, responseBody json.RawMessage, err error)
}

type httpGraphQLTransport struct {
	token      string
	httpClient *http.Client
}

func newHTTPGraphQLTransport(token string) httpGraphQLTransport {
	return httpGraphQLTransport{token: token, httpClient: &http.Client{}}
}

func (t httpGraphQLTransport) Post(ctx context.Context, body []byte) (int, json.RawMessage, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, graphqlEndpoint, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	request.Header.Set("Authorization", "Bearer "+t.token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", graphqlUserAgent)

	response, err := t.httpClient.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return response.StatusCode, nil, err
	}
	return response.StatusCode, responseBody, nil
}

type graphqlClient struct {
	transport graphqlTransport
}

type graphqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

type graphqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []graphqlError  `json:"errors"`
}

type graphqlError struct {
	Type       string `json:"type"`
	Message    string `json:"message"`
	Path       []any  `json:"path"`
	Extensions struct {
		Code string `json:"code"`
	} `json:"extensions"`
}

type repositoryError struct {
	alias     string
	errorType string
	message   string
}

func (e repositoryError) Error() string {
	return fmt.Sprintf("repository error on alias %s: %s", e.alias, describeError(e.errorType, e.message))
}

type pullRequestError struct {
	alias     string
	errorType string
	message   string
}

func (e pullRequestError) Error() string {
	return fmt.Sprintf("pull request error on alias %s: %s", e.alias, describeError(e.errorType, e.message))
}

type fieldError struct {
	alias     string
	path      []any
	errorType string
	message   string
}

func (e fieldError) Error() string {
	return fmt.Sprintf("field error on %s: %s", formatPath(e.path), describeError(e.errorType, e.message))
}

type queryError struct {
	errorType string
	code      string
	message   string
}

func (e queryError) Error() string {
	identifier := e.errorType
	if identifier == "" {
		identifier = e.code
	}
	return fmt.Sprintf("query error: %s", describeError(identifier, e.message))
}

type transportError struct {
	status int
	err    error
}

func (e transportError) Error() string {
	return fmt.Sprintf("GraphQL request failed with status %d: %v", e.status, e.err)
}

func (e transportError) Unwrap() error {
	return e.err
}

func describeError(identifier, message string) string {
	if identifier == "" {
		return message
	}
	return identifier + " " + message
}

func formatPath(path []any) string {
	return fmt.Sprintf("%v", path)
}

// Sends the query and decodes the response data into out, which is filled whenever the response
// carries decodable data - also when an error is returned, so that aliases which did resolve
// survive an error scoped to another one. Errors nested under one of the given aliases are
// returned as field errors; every other error fails the call as a typed error, the most severe
// one when there are several.
func (c graphqlClient) Do(
	ctx context.Context,
	query string,
	variables map[string]any,
	aliases []string,
	out any,
) ([]fieldError, error) {
	requestBody, err := json.Marshal(graphqlRequest{Query: query, Variables: variables})
	if err != nil {
		return nil, err
	}

	response, err := c.postWithRetry(ctx, requestBody)
	if err != nil {
		return nil, err
	}

	fieldErrors, hardError := classifyErrors(response.Errors, aliases)
	dataError := decodeData(response.Data, out)

	if hardError != nil {
		return fieldErrors, hardError
	}
	if dataError != nil {
		return nil, dataError
	}
	return fieldErrors, nil
}

func decodeData(data json.RawMessage, out any) error {
	if len(data) == 0 || string(data) == "null" {
		return queryError{message: "GraphQL response contained no data"}
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("error decoding GraphQL response data: %w", err)
	}
	return nil
}

type attempt struct {
	response  graphqlResponse
	err       error
	retryable bool
}

func (c graphqlClient) postWithRetry(ctx context.Context, requestBody []byte) (graphqlResponse, error) {
	for attemptNumber := 1; ; attemptNumber++ {
		result := c.post(ctx, requestBody)
		if result.err == nil {
			return result.response, nil
		}
		if !result.retryable || attemptNumber == graphqlMaxAttempts {
			return graphqlResponse{}, result.err
		}
		select {
		case <-ctx.Done():
			return graphqlResponse{}, transportError{err: ctx.Err()}
		case <-time.After(retryDelay):
		}
	}
}

func (c graphqlClient) post(ctx context.Context, requestBody []byte) attempt {
	status, responseBody, err := c.transport.Post(ctx, requestBody)
	if err != nil {
		return attempt{err: transportError{status: status, err: err}, retryable: true}
	}
	if status != http.StatusOK {
		return attempt{
			err:       transportError{status: status, err: fmt.Errorf("unexpected response: %s", responseBody)},
			retryable: status >= http.StatusInternalServerError || status == http.StatusTooManyRequests,
		}
	}

	var response graphqlResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return attempt{
			err:       transportError{status: status, err: fmt.Errorf("unparseable response: %w", err)},
			retryable: true,
		}
	}
	return attempt{response: response}
}

// GitHub decides the order of the errors array, so the returned hard error is the most severe
// one rather than the first one.
func classifyErrors(graphqlErrors []graphqlError, aliases []string) ([]fieldError, error) {
	var fieldErrors []fieldError
	var hardErrors []error

	for _, responseError := range graphqlErrors {
		classified, hardError := classifyError(responseError, aliases)
		if hardError != nil {
			hardErrors = append(hardErrors, hardError)
			continue
		}
		fieldErrors = append(fieldErrors, classified)
	}

	if len(hardErrors) == 0 {
		return fieldErrors, nil
	}
	return fieldErrors, slices.MaxFunc(hardErrors, func(a, b error) int {
		return cmp.Compare(errorSeverity(a), errorSeverity(b))
	})
}

func errorSeverity(err error) int {
	switch err.(type) {
	case queryError:
		return 3
	case repositoryError:
		return 2
	case pullRequestError:
		return 1
	}
	return 0
}

func classifyError(responseError graphqlError, aliases []string) (fieldError, error) {
	path := responseError.Path
	alias, isKnownAlias := rootAlias(path, aliases)
	if !isKnownAlias {
		return fieldError{}, queryError{
			errorType: responseError.Type,
			code:      responseError.Extensions.Code,
			message:   responseError.Message,
		}
	}
	if len(path) == 1 {
		return fieldError{}, repositoryError{
			alias:     alias,
			errorType: responseError.Type,
			message:   responseError.Message,
		}
	}
	if len(path) == 2 && path[1] == "pullRequest" {
		return fieldError{}, pullRequestError{
			alias:     alias,
			errorType: responseError.Type,
			message:   responseError.Message,
		}
	}
	return fieldError{
		alias:     alias,
		path:      path,
		errorType: responseError.Type,
		message:   responseError.Message,
	}, nil
}

func rootAlias(path []any, aliases []string) (string, bool) {
	if len(path) == 0 {
		return "", false
	}
	alias, isString := path[0].(string)
	if !isString || !slices.Contains(aliases, alias) {
		return "", false
	}
	return alias, true
}
