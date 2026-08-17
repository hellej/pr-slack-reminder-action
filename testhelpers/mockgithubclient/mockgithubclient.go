package mockgithubclient

import (
	"archive/zip"
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/go-github/v78/github"
	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/state"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

type MockGitHubClientOptions struct {
	PRsByNumber                map[int]*github.PullRequest
	ErrByPRNumber              map[int]error
	PRs                        []*github.PullRequest
	PRsByRepo                  map[string][]*github.PullRequest
	ListPRsResponseStatus      int
	ReviewsByPRNumber          map[int][]*github.PullRequestReview
	TimelineCommentsByPRNumber map[int][]*github.IssueComment
	PRServiceError             error
	IssueServiceError          error
	MockStateForUpdateMode     *state.State
	ListArtifactsError         error
	DownloadArtifactError      error
}

func MakeMockGitHubClientGetter(opts MockGitHubClientOptions) func(token, tokenForState string) githubclient.Client {
	if opts.ListPRsResponseStatus == 0 {
		opts.ListPRsResponseStatus = 200
	}

	return func(token, tokenForState string) githubclient.Client {
		mockHTTPClient := &mockHTTPClient{
			response: &http.Response{
				StatusCode: 200,
			},
			err:                    opts.DownloadArtifactError,
			mockStateForUpdateMode: opts.MockStateForUpdateMode,
		}
		mockActionsService := &mockActionsService{
			response: &github.Response{
				Response: &http.Response{
					StatusCode: 200,
				},
			},
			err:                    opts.ListArtifactsError,
			mockStateForUpdateMode: opts.MockStateForUpdateMode,
		}
		return githubclient.NewClient(
			mockHTTPClient,
			mockActionsService,
			NewGraphQLTransport(opts),
		)
	}
}

type UnusedGraphQLTransport struct{}

func (UnusedGraphQLTransport) Post(ctx context.Context, body []byte) (int, json.RawMessage, error) {
	return 0, nil, errors.New("unexpected GraphQL request")
}

func NewReview(login, name, state string, userType ...string) *github.PullRequestReview {
	return &github.PullRequestReview{
		User:  newUser(login, name, userType...),
		State: github.Ptr(state),
	}
}

func NewTimelineComment(login, name, body string, createdAt time.Time, userType ...string) *github.IssueComment {
	return &github.IssueComment{
		User:      newUser(login, name, userType...),
		Body:      github.Ptr(body),
		CreatedAt: &github.Timestamp{Time: createdAt},
	}
}

func newUser(login, name string, userType ...string) *github.User {
	var t *string
	if len(userType) > 0 && userType[0] != "" {
		t = github.Ptr(userType[0])
	}
	return &github.User{
		Login: github.Ptr(login),
		Name:  github.Ptr(name),
		Type:  t,
	}
}

// Renders the fixture options into GraphQL responses. The phase is read off the query text, and
// each alias is bound from the request variables: rN to ownerN/nameN, pN to ownerN/nameN/numN.
type GraphQLTransport struct {
	opts MockGitHubClientOptions
}

func NewGraphQLTransport(opts MockGitHubClientOptions) GraphQLTransport {
	return GraphQLTransport{opts: opts}
}

func (t GraphQLTransport) Post(ctx context.Context, body []byte) (int, json.RawMessage, error) {
	var request struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		return 0, nil, err
	}
	if strings.Contains(request.Query, "pullRequests(") {
		return t.openPRsResponse(request.Variables)
	}
	if strings.Contains(request.Query, "pullRequest(number:") {
		return t.enrichedPRsResponse(request.Variables)
	}
	return 0, nil, errors.New("unrecognized GraphQL query: " + request.Query)
}

type renderedResponse struct {
	Data   map[string]any  `json:"data"`
	Errors []renderedError `json:"errors,omitempty"`
}

type renderedError struct {
	Type    string `json:"type,omitempty"`
	Path    []any  `json:"path"`
	Message string `json:"message"`
}

const notFoundStatus = 404

// A 404 renders NOT_FOUND on the aliases of the repositories that have no PRs fixture; any other
// non-200 status fails the whole request at the transport level.
func (t GraphQLTransport) openPRsResponse(variables map[string]any) (int, json.RawMessage, error) {
	status := cmp.Or(t.opts.ListPRsResponseStatus, http.StatusOK)
	if status != http.StatusOK && status != notFoundStatus {
		body, err := json.Marshal(map[string]string{"message": errorMessage(t.opts.PRServiceError)})
		return status, body, err
	}

	response := renderedResponse{Data: map[string]any{"rateLimit": rateLimitJSON()}}
	for index, repoName := range postedRepositoryNames(variables) {
		alias := fmt.Sprintf("r%d", index)
		if status == notFoundStatus && !t.hasOpenPRsFixture(repoName) {
			response.Data[alias] = nil
			response.Errors = append(response.Errors, renderedError{
				Type:    "NOT_FOUND",
				Path:    []any{alias},
				Message: errorMessage(t.opts.PRServiceError),
			})
			continue
		}
		response.Data[alias] = map[string]any{
			"pullRequests": connectionJSON(utilities.Map(t.openPRs(repoName), openPullRequestNodeJSON)),
		}
	}
	return marshalResponse(response)
}

// An ErrByPRNumber entry renders as a PR-level error, a reference with no PR fixture as a
// PR-level NOT_FOUND, and an IssueServiceError as a field error on every PR's comments connection.
func (t GraphQLTransport) enrichedPRsResponse(variables map[string]any) (int, json.RawMessage, error) {
	response := renderedResponse{Data: map[string]any{"rateLimit": rateLimitJSON()}}
	for index, ref := range postedPullRequestRefs(variables) {
		alias := fmt.Sprintf("p%d", index)
		if err, hasError := t.opts.ErrByPRNumber[ref.number]; hasError {
			response.Data[alias] = map[string]any{"pullRequest": nil}
			response.Errors = append(response.Errors, renderedError{
				Path: []any{alias, "pullRequest"}, Message: err.Error(),
			})
			continue
		}
		pr := t.findPullRequest(ref)
		if pr == nil {
			response.Data[alias] = map[string]any{"pullRequest": nil}
			response.Errors = append(response.Errors, renderedError{
				Type: "NOT_FOUND",
				Path: []any{alias, "pullRequest"},
				Message: fmt.Sprintf(
					"Could not resolve to a PullRequest with the number of %d.", ref.number,
				),
			})
			continue
		}
		response.Data[alias] = map[string]any{"pullRequest": t.enrichedPullRequestNodeJSON(pr, ref.number)}
		if t.opts.IssueServiceError != nil {
			response.Errors = append(response.Errors, renderedError{
				Type:    "RESOURCE_LIMITS_EXCEEDED",
				Path:    []any{alias, "pullRequest", "comments"},
				Message: t.opts.IssueServiceError.Error(),
			})
		}
	}
	return marshalResponse(response)
}

func (t GraphQLTransport) openPRs(repoName string) []*github.PullRequest {
	if t.opts.PRsByRepo != nil {
		return t.opts.PRsByRepo[repoName]
	}
	return t.opts.PRs
}

func (t GraphQLTransport) hasOpenPRsFixture(repoName string) bool {
	if t.opts.PRsByRepo != nil {
		_, isConfigured := t.opts.PRsByRepo[repoName]
		return isConfigured
	}
	return len(t.opts.PRs) > 0
}

// PR scalars come from PRsByNumber when it is set, and from the listing fixtures otherwise.
func (t GraphQLTransport) findPullRequest(ref pullRequestRef) *github.PullRequest {
	if pr, isSet := t.opts.PRsByNumber[ref.number]; isSet {
		return pr
	}
	pr, _ := utilities.Find(t.openPRs(ref.repoName), func(pr *github.PullRequest) bool {
		return pr.GetNumber() == ref.number
	})
	return pr
}

// GetPRs selects state, merged and labels alongside the scalars both phases share.
func (t GraphQLTransport) enrichedPullRequestNodeJSON(
	pr *github.PullRequest, number int,
) map[string]any {
	node := pullRequestScalarsJSON(pr)
	node["number"] = number
	node["state"] = pullRequestNodeState(pr)
	node["merged"] = pr.GetMerged()
	node["labels"] = labelsJSON(pr)
	node["commits"] = connectionJSON([]map[string]any{})
	node["reviews"] = connectionJSON(utilities.Map(t.opts.ReviewsByPRNumber[number], reviewNodeJSON))
	node["comments"] = t.commentsJSON(number)
	return node
}

func pullRequestNodeState(pr *github.PullRequest) string {
	if pr.GetMerged() {
		return "MERGED"
	}
	if pr.GetState() == "closed" {
		return "CLOSED"
	}
	return "OPEN"
}

// The failed connection comes back null.
func (t GraphQLTransport) commentsJSON(number int) any {
	if t.opts.IssueServiceError != nil {
		return nil
	}
	return connectionJSON(utilities.Map(t.opts.TimelineCommentsByPRNumber[number], commentNodeJSON))
}

func openPullRequestNodeJSON(pr *github.PullRequest) map[string]any {
	node := pullRequestScalarsJSON(pr)
	node["labels"] = labelsJSON(pr)
	return node
}

func labelsJSON(pr *github.PullRequest) map[string]any {
	return connectionJSON(utilities.Map(pr.Labels, labelNodeJSON))
}

func pullRequestScalarsJSON(pr *github.PullRequest) map[string]any {
	return map[string]any{
		"number":     pr.GetNumber(),
		"title":      pr.GetTitle(),
		"url":        pr.GetHTMLURL(),
		"isDraft":    pr.GetDraft(),
		"createdAt":  pr.GetCreatedAt().Time,
		"updatedAt":  pr.GetUpdatedAt().Time,
		"headRefOid": pr.GetHead().GetSHA(),
		"author":     authorNodeJSON(pr.GetUser()),
	}
}

func labelNodeJSON(label *github.Label) map[string]any {
	return map[string]any{"name": label.GetName()}
}

func reviewNodeJSON(review *github.PullRequestReview) map[string]any {
	return map[string]any{
		"state":  review.GetState(),
		"author": authorNodeJSON(review.GetUser()),
	}
}

func commentNodeJSON(comment *github.IssueComment) map[string]any {
	return map[string]any{
		"createdAt": comment.GetCreatedAt().Time,
		"body":      comment.GetBody(),
		"author":    authorNodeJSON(comment.GetUser()),
	}
}

// An unset user type renders as "User" - anything else drops the name in the collaborator mapper.
func authorNodeJSON(user *github.User) map[string]any {
	if user == nil {
		return nil
	}
	return map[string]any{
		"login":      user.GetLogin(),
		"__typename": cmp.Or(user.GetType(), "User"),
		"name":       user.GetName(),
	}
}

func connectionJSON[T any](nodes []T) map[string]any {
	return map[string]any{"nodes": nodes}
}

func rateLimitJSON() map[string]any {
	return map[string]any{"cost": 1, "remaining": 4999, "limit": 5000}
}

func marshalResponse(response renderedResponse) (int, json.RawMessage, error) {
	body, err := json.Marshal(response)
	if err != nil {
		return 0, nil, err
	}
	return http.StatusOK, body, nil
}

func errorMessage(err error) string {
	if err == nil {
		return "GraphQL request failed"
	}
	return err.Error()
}

type pullRequestRef struct {
	repoName string
	number   int
}

func postedRepositoryNames(variables map[string]any) []string {
	names := []string{}
	for index := 0; ; index++ {
		name, isSet := variables[fmt.Sprintf("name%d", index)].(string)
		if !isSet {
			return names
		}
		names = append(names, name)
	}
}

func postedPullRequestRefs(variables map[string]any) []pullRequestRef {
	refs := []pullRequestRef{}
	for index, repoName := range postedRepositoryNames(variables) {
		number, isSet := variables[fmt.Sprintf("num%d", index)].(float64)
		if !isSet {
			return refs
		}
		refs = append(refs, pullRequestRef{repoName: repoName, number: int(number)})
	}
	return refs
}

type mockActionsService struct {
	response               *github.Response
	err                    error
	mockStateForUpdateMode *state.State
}

func (m *mockActionsService) ListArtifacts(
	ctx context.Context, owner string, repo string, opts *github.ListArtifactsOptions,
) (*github.ArtifactList, *github.Response, error) {
	if m.err != nil {
		return nil, m.response, m.err
	}

	artifacts := []*github.Artifact{}
	if m.mockStateForUpdateMode != nil {
		artifacts = append(artifacts, &github.Artifact{
			ID:        github.Ptr(int64(123)),
			Name:      github.Ptr("pr-slack-reminder-state"),
			CreatedAt: &github.Timestamp{Time: time.Now().Add(-1 * time.Hour)},
		})
	}

	return &github.ArtifactList{
		TotalCount: github.Ptr(int64(len(artifacts))),
		Artifacts:  artifacts,
	}, m.response, nil
}

func (m *mockActionsService) DownloadArtifact(
	ctx context.Context, owner, repo string, artifactID int64, maxRedirects int,
) (*url.URL, *github.Response, error) {
	if m.err != nil {
		return nil, m.response, m.err
	}
	u, _ := url.Parse("https://example.com/mock-download-url")
	return u, m.response, nil
}

type mockHTTPClient struct {
	response               *http.Response
	err                    error
	mockStateForUpdateMode *state.State
}

func (m *mockHTTPClient) Get(url string) (*http.Response, error) {
	if m.err != nil {
		return m.response, m.err
	}

	if url == "https://example.com/mock-download-url" && m.mockStateForUpdateMode != nil {
		zipData, err := createMockArtifactZip(m.mockStateForUpdateMode)
		if err != nil {
			return nil, err
		}

		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader(zipData)),
		}, nil
	}

	return m.response, m.err
}

func createMockArtifactZip(mockState *state.State) ([]byte, error) {
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	file, err := zipWriter.Create("pr-slack-reminder-state.json")
	if err != nil {
		return nil, err
	}

	stateJSON, err := json.Marshal(mockState)
	if err != nil {
		return nil, err
	}

	if _, err := file.Write(stateJSON); err != nil {
		return nil, err
	}

	if err := zipWriter.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
