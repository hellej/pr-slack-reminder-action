package mockgithubclient

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/go-github/v78/github"
	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/state"
)

type MockGitHubClientOptions struct {
	PRsByNumber            map[int]*github.PullRequest
	ErrByPRNumber          map[int]error
	PRs                    []*github.PullRequest
	PRsByRepo              map[string][]*github.PullRequest
	ListPRsResponseStatus  int
	ReviewsByPRNumber      map[int][]*github.PullRequestReview
	CommentsByPRNumber     map[int][]*github.PullRequestComment
	PRServiceError         error
	IssueServiceError      error
	MockStateForUpdateMode *state.State
	ListArtifactsError     error
	DownloadArtifactError  error
}

func MakeMockGitHubClientGetter(opts MockGitHubClientOptions) func(token string) githubclient.Client {
	if opts.ListPRsResponseStatus == 0 {
		opts.ListPRsResponseStatus = 200
	}

	return func(token string) githubclient.Client {
		mockPRService := &mockPullRequestService{
			prsByNumber:        opts.PRsByNumber,
			errorByPRNumber:    opts.ErrByPRNumber,
			prs:                opts.PRs,
			prsByRepo:          opts.PRsByRepo,
			reviewsByPRNumber:  opts.ReviewsByPRNumber,
			commentsByPRNumber: opts.CommentsByPRNumber,
			response: &github.Response{
				Response: &http.Response{
					StatusCode: opts.ListPRsResponseStatus,
				},
			},
			err: opts.PRServiceError,
		}
		mockIssueService := &mockIssueService{
			mockTimelineCommentsByPRNumber: map[int][]*github.IssueComment{},
			response: &github.Response{
				Response: &http.Response{
					StatusCode: 200,
				},
			},
			err: opts.IssueServiceError,
		}
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
		return githubclient.NewClient(mockHTTPClient, mockPRService, mockIssueService, mockActionsService)
	}
}

func NewReview(id int64, state, login, name, body string, userType ...string) *github.PullRequestReview {
	var t *string
	if len(userType) > 0 && userType[0] != "" {
		t = github.Ptr(userType[0])
	}
	var b *string
	if body != "" {
		b = github.Ptr(body)
	}
	return &github.PullRequestReview{
		ID:   github.Ptr(id),
		Body: b,
		User: &github.User{
			Login: github.Ptr(login),
			Name:  github.Ptr(name),
			Type:  t,
		},
		State: github.Ptr(state),
	}
}

func NewComment(id int64, login, name, body string, userType ...string) *github.PullRequestComment {
	var t *string
	if len(userType) > 0 && userType[0] != "" {
		t = github.Ptr(userType[0])
	}
	var b *string
	if body != "" {
		b = github.Ptr(body)
	}
	return &github.PullRequestComment{
		ID:   github.Ptr(id),
		Body: b,
		User: &github.User{
			Login: github.Ptr(login),
			Name:  github.Ptr(name),
			Type:  t,
		},
	}
}

type mockPullRequestService struct {
	prsByNumber        map[int]*github.PullRequest
	errorByPRNumber    map[int]error
	prs                []*github.PullRequest
	prsByRepo          map[string][]*github.PullRequest
	reviewsByPRNumber  map[int][]*github.PullRequestReview
	commentsByPRNumber map[int][]*github.PullRequestComment
	response           *github.Response
	err                error
}

func (m *mockPullRequestService) Get(
	ctx context.Context, owner string, repo string, number int,
) (*github.PullRequest, *github.Response, error) {
	if err, ok := m.errorByPRNumber[number]; ok {
		return nil, m.response, err
	}
	if pr, ok := m.prsByNumber[number]; ok {
		return pr, m.response, m.err
	}
	return nil, m.response, m.err
}

func (m *mockPullRequestService) List(
	ctx context.Context, owner string, repo string, opts *github.PullRequestListOptions,
) ([]*github.PullRequest, *github.Response, error) {
	if m.prsByRepo != nil {
		return m.prsByRepo[repo], m.response, m.err
	}
	return m.prs, m.response, m.err
}

func (m *mockPullRequestService) ListReviews(
	ctx context.Context, owner string, repo string, number int, opts *github.ListOptions,
) ([]*github.PullRequestReview, *github.Response, error) {
	reviews := m.reviewsByPRNumber[number]
	return reviews, m.response, m.err
}

func (m *mockPullRequestService) ListComments(
	ctx context.Context, owner string, repo string, number int, opts *github.PullRequestListCommentsOptions,
) ([]*github.PullRequestComment, *github.Response, error) {
	comments := m.commentsByPRNumber[number]
	return comments, m.response, m.err
}

type mockIssueService struct {
	mockTimelineCommentsByPRNumber map[int][]*github.IssueComment
	response                       *github.Response
	err                            error
}

func (m *mockIssueService) ListComments(
	ctx context.Context, owner string, repo string, number int, opts *github.IssueListCommentsOptions,
) ([]*github.IssueComment, *github.Response, error) {
	comments := m.mockTimelineCommentsByPRNumber[number]
	return comments, m.response, m.err
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
