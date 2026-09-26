package githubclient

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/google/go-github/v78/github"
)

type testState struct {
	Version int    `json:"version"`
	Message string `json:"message"`
}

type mockActionsServiceWithArtifacts struct {
	artifacts      []*github.Artifact
	downloadURL    *url.URL
	listError      error
	downloadError  error
	mockHTTPClient *mockHTTPClientWithZip
}

func (m *mockActionsServiceWithArtifacts) ListArtifacts(
	ctx context.Context, owner string, repo string, opts *github.ListArtifactsOptions,
) (*github.ArtifactList, *github.Response, error) {
	if m.listError != nil {
		return nil, &github.Response{
			Response: &http.Response{StatusCode: 500, Status: "500 Internal Server Error"},
		}, m.listError
	}
	return &github.ArtifactList{
		TotalCount: github.Ptr(int64(len(m.artifacts))),
		Artifacts:  m.artifacts,
	}, &github.Response{Response: &http.Response{StatusCode: 200}}, nil
}

func (m *mockActionsServiceWithArtifacts) DownloadArtifact(
	ctx context.Context, owner string, repo string, artifactID int64, maxRedirects int,
) (*url.URL, *github.Response, error) {
	if m.downloadError != nil {
		return nil, &github.Response{Response: &http.Response{StatusCode: 500}}, m.downloadError
	}
	return m.downloadURL, &github.Response{Response: &http.Response{StatusCode: 200}}, nil
}

type mockHTTPClientWithZip struct {
	zipData       []byte
	statusCode    int
	err           error
	bodyReadError error
}

func (m *mockHTTPClientWithZip) Do(request *http.Request) (*http.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	var body io.Reader = bytes.NewReader(m.zipData)
	if m.bodyReadError != nil {
		body = iotest.ErrReader(m.bodyReadError)
	}
	return &http.Response{StatusCode: m.statusCode, Body: io.NopCloser(body)}, nil
}

func createTestZip(filename string, content []byte) ([]byte, error) {
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	file, err := zipWriter.Create(filename)
	if err != nil {
		return nil, err
	}

	if _, err := file.Write(content); err != nil {
		return nil, err
	}

	if err := zipWriter.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func TestFetchLatestArtifactByName(t *testing.T) {
	withoutRetryWaits(t)

	tests := []struct {
		name                  string
		artifactName          string
		jsonFilePath          string
		artifacts             []*github.Artifact
		zipFilename           string
		zipContent            testState
		zipJSON               string // replaces zipContent, for JSON that testState can't marshal to
		zipData               []byte // replaces the whole zip, when zipFilename is empty
		listError             error
		downloadError         error
		httpError             error
		bodyReadError         error
		httpStatus            int
		expectedData          testState
		expectError           bool
		errorContains         string
		expectNoArtifactFound bool
	}{
		{
			name:         "successful fetch with exact filename match",
			artifactName: "test-artifact",
			jsonFilePath: "state.json",
			artifacts: []*github.Artifact{
				{
					ID:        github.Ptr(int64(123)),
					Name:      github.Ptr("test-artifact"),
					CreatedAt: &github.Timestamp{Time: time.Now()},
				},
			},
			zipFilename:  "state.json",
			zipContent:   testState{Version: 1, Message: "test data"},
			httpStatus:   200,
			expectedData: testState{Version: 1, Message: "test data"},
			expectError:  false,
		},
		{
			name:         "successful fetch with path in jsonFilePath",
			artifactName: "test-artifact",
			jsonFilePath: "/tmp/state.json",
			artifacts: []*github.Artifact{
				{
					ID:        github.Ptr(int64(123)),
					Name:      github.Ptr("test-artifact"),
					CreatedAt: &github.Timestamp{Time: time.Now()},
				},
			},
			zipFilename:  "state.json",
			zipContent:   testState{Version: 2, Message: "path test"},
			httpStatus:   200,
			expectedData: testState{Version: 2, Message: "path test"},
			expectError:  false,
		},
		{
			name:         "successful fetch with path in zip file",
			artifactName: "test-artifact",
			jsonFilePath: "state.json",
			artifacts: []*github.Artifact{
				{
					ID:        github.Ptr(int64(123)),
					Name:      github.Ptr("test-artifact"),
					CreatedAt: &github.Timestamp{Time: time.Now()},
				},
			},
			zipFilename:  "path/to/state.json",
			zipContent:   testState{Version: 3, Message: "nested path"},
			httpStatus:   200,
			expectedData: testState{Version: 3, Message: "nested path"},
			expectError:  false,
		},
		{
			name:         "multiple artifacts returns latest",
			artifactName: "test-artifact",
			jsonFilePath: "state.json",
			artifacts: []*github.Artifact{
				{
					ID:        github.Ptr(int64(123)),
					Name:      github.Ptr("test-artifact"),
					CreatedAt: &github.Timestamp{Time: time.Now().Add(-2 * time.Hour)},
				},
				{
					ID:        github.Ptr(int64(456)),
					Name:      github.Ptr("test-artifact"),
					CreatedAt: &github.Timestamp{Time: time.Now().Add(-1 * time.Hour)},
				},
			},
			zipFilename:  "state.json",
			zipContent:   testState{Version: 4, Message: "latest"},
			httpStatus:   200,
			expectedData: testState{Version: 4, Message: "latest"},
			expectError:  false,
		},
		{
			name:                  "no artifacts found",
			artifactName:          "missing-artifact",
			jsonFilePath:          "state.json",
			artifacts:             []*github.Artifact{},
			expectError:           true,
			errorContains:         "no artifacts found with name \"missing-artifact\"",
			expectNoArtifactFound: true,
		},
		{
			name:          "list artifacts error",
			artifactName:  "test-artifact",
			jsonFilePath:  "state.json",
			listError:     fmt.Errorf("403 Forbidden"),
			expectError:   true,
			errorContains: "failed to list artifacts: 403 Forbidden status=500 Internal Server Error",
		},
		{
			name:         "download artifact error",
			artifactName: "test-artifact",
			jsonFilePath: "state.json",
			artifacts: []*github.Artifact{
				{
					ID:        github.Ptr(int64(123)),
					Name:      github.Ptr("test-artifact"),
					CreatedAt: &github.Timestamp{Time: time.Now()},
				},
			},
			downloadError: fmt.Errorf("download failed"),
			expectError:   true,
			errorContains: "get artifact download URL",
		},
		{
			name:         "HTTP error downloading zip",
			artifactName: "test-artifact",
			jsonFilePath: "state.json",
			artifacts: []*github.Artifact{
				{
					ID:        github.Ptr(int64(123)),
					Name:      github.Ptr("test-artifact"),
					CreatedAt: &github.Timestamp{Time: time.Now()},
				},
			},
			httpError:     fmt.Errorf("network error"),
			expectError:   true,
			errorContains: "download artifact zip",
		},
		{
			name:         "HTTP status error",
			artifactName: "test-artifact",
			jsonFilePath: "state.json",
			artifacts: []*github.Artifact{
				{
					ID:        github.Ptr(int64(123)),
					Name:      github.Ptr("test-artifact"),
					CreatedAt: &github.Timestamp{Time: time.Now()},
				},
			},
			zipFilename:   "state.json",
			zipContent:    testState{Version: 1, Message: "test"},
			httpStatus:    404,
			expectError:   true,
			errorContains: "unexpected status code 404",
		},
		{
			name:         "file not found in zip",
			artifactName: "test-artifact",
			jsonFilePath: "missing.json",
			artifacts: []*github.Artifact{
				{
					ID:        github.Ptr(int64(123)),
					Name:      github.Ptr("test-artifact"),
					CreatedAt: &github.Timestamp{Time: time.Now()},
				},
			},
			zipFilename:   "state.json",
			zipContent:    testState{Version: 1, Message: "test"},
			httpStatus:    200,
			expectError:   true,
			errorContains: "not found inside artifact zip",
		},
		{
			name:         "download interrupted while reading the zip",
			artifactName: "test-artifact",
			jsonFilePath: "state.json",
			artifacts: []*github.Artifact{
				{
					ID:        github.Ptr(int64(123)),
					Name:      github.Ptr("test-artifact"),
					CreatedAt: &github.Timestamp{Time: time.Now()},
				},
			},
			bodyReadError: errors.New("connection reset"),
			httpStatus:    200,
			expectError:   true,
			errorContains: "read artifact zip: connection reset",
		},
		{
			name:         "downloaded file is not a zip",
			artifactName: "test-artifact",
			jsonFilePath: "state.json",
			artifacts: []*github.Artifact{
				{
					ID:        github.Ptr(int64(123)),
					Name:      github.Ptr("test-artifact"),
					CreatedAt: &github.Timestamp{Time: time.Now()},
				},
			},
			zipData:       []byte("not a zip"),
			httpStatus:    200,
			expectError:   true,
			errorContains: "open zip",
		},
		{
			name:         "JSON file does not match the target type",
			artifactName: "test-artifact",
			jsonFilePath: "state.json",
			artifacts: []*github.Artifact{
				{
					ID:        github.Ptr(int64(123)),
					Name:      github.Ptr("test-artifact"),
					CreatedAt: &github.Timestamp{Time: time.Now()},
				},
			},
			zipFilename:   "state.json",
			zipJSON:       `{"version":"one"}`,
			httpStatus:    200,
			expectError:   true,
			errorContains: `decode json "state.json"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zipData := tt.zipData
			var err error

			if tt.zipFilename != "" {
				jsonContent := []byte(tt.zipJSON)
				if tt.zipJSON == "" {
					jsonContent, _ = json.Marshal(tt.zipContent)
				}
				zipData, err = createTestZip(tt.zipFilename, jsonContent)
				if err != nil {
					t.Fatalf("Failed to create test zip: %v", err)
				}
			}

			mockHTTPClient := &mockHTTPClientWithZip{
				zipData:       zipData,
				statusCode:    tt.httpStatus,
				err:           tt.httpError,
				bodyReadError: tt.bodyReadError,
			}

			downloadURL, _ := url.Parse("https://example.com/download")
			mockActions := &mockActionsServiceWithArtifacts{
				artifacts:      tt.artifacts,
				downloadURL:    downloadURL,
				listError:      tt.listError,
				downloadError:  tt.downloadError,
				mockHTTPClient: mockHTTPClient,
			}

			client := NewClient(mockHTTPClient, mockActions, nil)

			var result testState
			err = client.FetchLatestArtifactByName(
				context.Background(),
				"test-owner",
				"test-repo",
				tt.artifactName,
				tt.jsonFilePath,
				&result,
			)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error containing %q, got nil", tt.errorContains)
					return
				}
				if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error containing %q, got %q", tt.errorContains, err.Error())
				}
				if errors.Is(err, ErrNoArtifactFound) != tt.expectNoArtifactFound {
					t.Errorf("Expected errors.Is(err, ErrNoArtifactFound) to be %v, got %q", tt.expectNoArtifactFound, err)
				}
				return
			}

			if err != nil {
				t.Errorf("Expected no error, got: %v", err)
				return
			}

			if result.Version != tt.expectedData.Version || result.Message != tt.expectedData.Message {
				t.Errorf("Expected data %+v, got %+v", tt.expectedData, result)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// One scripted GitHub API answer. A zero status is no response at all, as on a network error.
type githubCallResult struct {
	status int
	err    error
}

func (r githubCallResult) response() *github.Response {
	if r.status == 0 {
		return nil
	}
	return &github.Response{Response: &http.Response{StatusCode: r.status}}
}

// Answers each call with the next scripted result, repeating the last one, and succeeds once the
// script runs out. Every download URL is new, so a test can tell a fresh URL from a reused one.
type scriptedActionsService struct {
	listResults     []githubCallResult
	downloadResults []githubCallResult
	artifacts       []*github.Artifact
	listCalls       int
	downloadCalls   int
}

func (s *scriptedActionsService) ListArtifacts(
	ctx context.Context, owner string, repo string, opts *github.ListArtifactsOptions,
) (*github.ArtifactList, *github.Response, error) {
	s.listCalls++
	if result, isScripted := scriptedResult(s.listResults, s.listCalls); isScripted {
		return nil, result.response(), result.err
	}
	return &github.ArtifactList{
		TotalCount: github.Ptr(int64(len(s.artifacts))),
		Artifacts:  s.artifacts,
	}, githubCallResult{status: 200}.response(), nil
}

func (s *scriptedActionsService) DownloadArtifact(
	ctx context.Context, owner string, repo string, artifactID int64, maxRedirects int,
) (*url.URL, *github.Response, error) {
	s.downloadCalls++
	if result, isScripted := scriptedResult(s.downloadResults, s.downloadCalls); isScripted {
		return nil, result.response(), result.err
	}
	downloadURL, _ := url.Parse(fmt.Sprintf("https://example.com/download/%d", s.downloadCalls))
	return downloadURL, githubCallResult{status: 302}.response(), nil
}

func scriptedResult(results []githubCallResult, callNumber int) (githubCallResult, bool) {
	if callNumber > len(results) || results[callNumber-1].err == nil {
		return githubCallResult{}, false
	}
	return results[callNumber-1], true
}

type zipDownloadResult struct {
	status        int
	err           error
	bodyReadError error
	hangs         bool
	bodyHangs     bool
}

// Serves the zip once the script runs out. A hanging attempt blocks until its request's ctx is
// done, or fails the test after a second if that ctx never ends.
type scriptedZipClient struct {
	t                  *testing.T
	results            []zipDownloadResult
	zipData            []byte
	requestedURLs      []string
	requestHasDeadline []bool
}

func (c *scriptedZipClient) Do(request *http.Request) (*http.Response, error) {
	c.requestedURLs = append(c.requestedURLs, request.URL.String())
	_, hasDeadline := request.Context().Deadline()
	c.requestHasDeadline = append(c.requestHasDeadline, hasDeadline)

	result := zipDownloadResult{status: 200}
	if len(c.requestedURLs) <= len(c.results) {
		result = c.results[len(c.requestedURLs)-1]
	}
	if result.hangs {
		return nil, c.waitUntilDone(request.Context())
	}
	if result.err != nil {
		return nil, result.err
	}
	var body io.Reader = bytes.NewReader(c.zipData)
	if result.bodyReadError != nil {
		body = iotest.ErrReader(result.bodyReadError)
	}
	if result.bodyHangs {
		body = iotest.ErrReader(c.waitUntilDone(request.Context()))
	}
	return &http.Response{StatusCode: result.status, Body: io.NopCloser(body)}, nil
}

func (c *scriptedZipClient) waitUntilDone(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Second):
		c.t.Error("expected the request's ctx to end the hanging attempt")
		return errors.New("hung past its attempt deadline")
	}
}

func TestFetchLatestArtifactByNameRetriesTransientFailures(t *testing.T) {
	noResponse := githubCallResult{err: errors.New("connection reset")}
	serverError := githubCallResult{status: 500, err: errors.New("500 Internal Server Error")}

	tests := []struct {
		name                  string
		listResults           []githubCallResult
		downloadResults       []githubCallResult
		zipResults            []zipDownloadResult
		noArtifacts           bool
		zipData               []byte
		expectError           bool
		expectedListCalls     int
		expectedDownloadCalls int
		expectedZipDownloads  int
		expectedWaits         []time.Duration
		expectedRetryLog      string
	}{
		{
			name:                  "artifact list server error then success",
			listResults:           []githubCallResult{serverError},
			expectedListCalls:     2,
			expectedDownloadCalls: 1,
			expectedZipDownloads:  1,
			expectedWaits:         []time.Duration{2 * time.Second},
			expectedRetryLog:      "artifact list attempt 1 failed, retrying in 2s: failed to list artifacts: 500 Internal Server Error\n",
		},
		{
			name:              "artifact list without a response fails after 3 attempts",
			listResults:       []githubCallResult{noResponse, noResponse, noResponse, noResponse},
			expectError:       true,
			expectedListCalls: 3,
			expectedWaits:     []time.Duration{2 * time.Second, 5 * time.Second},
			expectedRetryLog:  "artifact list attempt 2 failed, retrying in 5s: failed to list artifacts: connection reset",
		},
		{
			name:              "artifact list client error just below the server errors is not retried",
			listResults:       []githubCallResult{{status: 499, err: errors.New("499")}},
			expectError:       true,
			expectedListCalls: 1,
		},
		{
			name:              "artifact list rate limit 403 is not retried",
			listResults:       []githubCallResult{{status: 403, err: errors.New("API rate limit exceeded")}},
			expectError:       true,
			expectedListCalls: 1,
		},
		{
			name:              "artifact list 429 is not retried",
			listResults:       []githubCallResult{{status: 429, err: errors.New("Too Many Requests")}},
			expectError:       true,
			expectedListCalls: 1,
		},
		{
			name:              "no artifact found is not retried",
			noArtifacts:       true,
			expectError:       true,
			expectedListCalls: 1,
		},
		{
			name:                  "download URL server error then success",
			downloadResults:       []githubCallResult{serverError},
			expectedListCalls:     1,
			expectedDownloadCalls: 2,
			expectedZipDownloads:  1,
			expectedWaits:         []time.Duration{2 * time.Second},
			expectedRetryLog:      "artifact download attempt 1 failed, retrying in 2s: get artifact download URL: 500 Internal Server Error",
		},
		{
			name:                  "download URL without a response then success",
			downloadResults:       []githubCallResult{noResponse},
			expectedListCalls:     1,
			expectedDownloadCalls: 2,
			expectedZipDownloads:  1,
			expectedWaits:         []time.Duration{2 * time.Second},
		},
		{
			name:                  "download URL for an expired artifact is not retried",
			downloadResults:       []githubCallResult{{status: 410, err: errors.New("unexpected status code: 410 Gone")}},
			expectError:           true,
			expectedListCalls:     1,
			expectedDownloadCalls: 1,
		},
		{
			name:                  "zip download network error retries with a fresh download URL",
			zipResults:            []zipDownloadResult{{err: errors.New("connection reset")}},
			expectedListCalls:     1,
			expectedDownloadCalls: 2,
			expectedZipDownloads:  2,
			expectedWaits:         []time.Duration{2 * time.Second},
			expectedRetryLog:      "artifact download attempt 1 failed, retrying in 2s: download artifact zip: connection reset",
		},
		{
			name:                  "zip download server error then success",
			zipResults:            []zipDownloadResult{{status: 500}},
			expectedListCalls:     1,
			expectedDownloadCalls: 2,
			expectedZipDownloads:  2,
			expectedWaits:         []time.Duration{2 * time.Second},
		},
		{
			name:                  "zip download failing to read the body then success",
			zipResults:            []zipDownloadResult{{status: 200, bodyReadError: errors.New("unexpected EOF")}},
			expectedListCalls:     1,
			expectedDownloadCalls: 2,
			expectedZipDownloads:  2,
			expectedWaits:         []time.Duration{2 * time.Second},
		},
		{
			name:                  "zip download server errors fail after 3 attempts",
			zipResults:            []zipDownloadResult{{status: 503}, {status: 503}, {status: 503}, {status: 503}},
			expectError:           true,
			expectedListCalls:     1,
			expectedDownloadCalls: 3,
			expectedZipDownloads:  3,
			expectedWaits:         []time.Duration{2 * time.Second, 5 * time.Second},
		},
		{
			name:                  "zip download client error just below the server errors is not retried",
			zipResults:            []zipDownloadResult{{status: 499}},
			expectError:           true,
			expectedListCalls:     1,
			expectedDownloadCalls: 1,
			expectedZipDownloads:  1,
		},
		{
			name:                  "zip that does not open is not retried",
			zipData:               []byte("not a zip"),
			expectError:           true,
			expectedListCalls:     1,
			expectedDownloadCalls: 1,
			expectedZipDownloads:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := withoutRetryWaits(t)
			logOutput := captureLog(t)
			actions := &scriptedActionsService{
				listResults:     tt.listResults,
				downloadResults: tt.downloadResults,
				artifacts:       testArtifacts(tt.noArtifacts),
			}
			zipClient := &scriptedZipClient{t: t, results: tt.zipResults, zipData: tt.zipData}
			if tt.zipData == nil {
				zipClient.zipData = stateZip(t, testState{Version: 7, Message: "loaded"})
			}

			var loaded testState
			err := NewClient(zipClient, actions, nil).FetchLatestArtifactByName(
				context.Background(), "test-owner", "test-repo", "test-artifact", "state.json", &loaded,
			)

			if tt.expectError != (err != nil) {
				t.Fatalf("expected an error: %v, got %v", tt.expectError, err)
			}
			if !tt.expectError && loaded.Version != 7 {
				t.Errorf("expected the state to load, got %+v", loaded)
			}
			if actions.listCalls != tt.expectedListCalls {
				t.Errorf("expected %d artifact list calls, got %d", tt.expectedListCalls, actions.listCalls)
			}
			if actions.downloadCalls != tt.expectedDownloadCalls {
				t.Errorf("expected %d download URL calls, got %d", tt.expectedDownloadCalls, actions.downloadCalls)
			}
			if len(zipClient.requestedURLs) != tt.expectedZipDownloads {
				t.Errorf("expected %d zip downloads, got %d", tt.expectedZipDownloads, len(zipClient.requestedURLs))
			}
			if len(slices.Compact(slices.Sorted(slices.Values(zipClient.requestedURLs)))) != len(zipClient.requestedURLs) {
				t.Errorf("expected a fresh download URL for each zip download, got %v", zipClient.requestedURLs)
			}
			if !slices.Equal(recorder.requestedWaits(), tt.expectedWaits) {
				t.Errorf("expected waits %v, got %v", tt.expectedWaits, recorder.requestedWaits())
			}
			if !strings.Contains(logOutput.String(), tt.expectedRetryLog) {
				t.Errorf("expected a log line containing %q, got %q", tt.expectedRetryLog, logOutput.String())
			}
		})
	}
}

func TestFetchLatestArtifactByNameRetriesAZipDownloadCutOffByItsDeadline(t *testing.T) {
	tests := []struct {
		name       string
		zipResults []zipDownloadResult
	}{
		{name: "response never arrives", zipResults: []zipDownloadResult{{hangs: true}}},
		{name: "body read never ends", zipResults: []zipDownloadResult{{status: 200, bodyHangs: true}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withoutRetryWaits(t)
			withAttemptTimeout(t, 20*time.Millisecond)
			actions := &scriptedActionsService{artifacts: testArtifacts(false)}
			zipClient := &scriptedZipClient{
				t:       t,
				results: tt.zipResults,
				zipData: stateZip(t, testState{Version: 7, Message: "loaded"}),
			}

			var loaded testState
			err := NewClient(zipClient, actions, nil).FetchLatestArtifactByName(
				context.Background(), "test-owner", "test-repo", "test-artifact", "state.json", &loaded,
			)

			if err != nil || loaded.Version != 7 {
				t.Fatalf("expected the second attempt to load the state, got %+v and %v", loaded, err)
			}
			if !slices.Equal(zipClient.requestHasDeadline, []bool{true, true}) {
				t.Errorf("expected 2 zip requests, each carrying a deadline, got %v", zipClient.requestHasDeadline)
			}
		})
	}
}

func testArtifacts(none bool) []*github.Artifact {
	if none {
		return []*github.Artifact{}
	}
	return []*github.Artifact{{
		ID:        github.Ptr(int64(123)),
		Name:      github.Ptr("test-artifact"),
		CreatedAt: &github.Timestamp{Time: time.Now()},
	}}
}

func stateZip(t *testing.T, content testState) []byte {
	t.Helper()
	jsonContent, _ := json.Marshal(content)
	zipData, err := createTestZip("state.json", jsonContent)
	if err != nil {
		t.Fatalf("failed to create test zip: %v", err)
	}
	return zipData
}
