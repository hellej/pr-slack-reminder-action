package githubclient

import (
	"archive/zip"
	"bytes"
	"cmp"
	"context"
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

type githubCallResult struct {
	status int
	err    error
}

func (r githubCallResult) response() *github.Response {
	if r.status == 0 {
		return nil
	}
	return &github.Response{Response: &http.Response{
		StatusCode: r.status,
		Status:     strings.TrimSpace(fmt.Sprintf("%d %s", r.status, http.StatusText(r.status))),
	}}
}

// Call N fails with the Nth scripted result when that result carries an error, and succeeds
// otherwise, past the script's end too. Every download URL is new, so a test can tell a fresh
// URL from a reused one.
type scriptedActionsService struct {
	listResults           []githubCallResult
	downloadResults       []githubCallResult
	artifacts             []*github.Artifact
	listCalls             int
	downloadCalls         int
	downloadedArtifactIDs []int64
	listCallDeadlines     []time.Time
	downloadCallDeadlines []time.Time
}

func (s *scriptedActionsService) ListArtifacts(
	ctx context.Context, owner string, repo string, opts *github.ListArtifactsOptions,
) (*github.ArtifactList, *github.Response, error) {
	s.listCalls++
	s.listCallDeadlines = append(s.listCallDeadlines, deadlineOf(ctx))
	if result, isScripted := scriptedFailure(s.listResults, s.listCalls); isScripted {
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
	s.downloadCallDeadlines = append(s.downloadCallDeadlines, deadlineOf(ctx))
	s.downloadedArtifactIDs = append(s.downloadedArtifactIDs, artifactID)
	if result, isScripted := scriptedFailure(s.downloadResults, s.downloadCalls); isScripted {
		return nil, result.response(), result.err
	}
	downloadURL, _ := url.Parse(fmt.Sprintf("https://example.com/download/%d", s.downloadCalls))
	return downloadURL, githubCallResult{status: 302}.response(), nil
}

func scriptedFailure(results []githubCallResult, callNumber int) (githubCallResult, bool) {
	if callNumber > len(results) || results[callNumber-1].err == nil {
		return githubCallResult{}, false
	}
	return results[callNumber-1], true
}

func deadlineOf(ctx context.Context) time.Time {
	deadline, _ := ctx.Deadline()
	return deadline
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
	t                *testing.T
	results          []zipDownloadResult
	zipData          []byte
	requestedURLs    []string
	requestDeadlines []time.Time
}

func (c *scriptedZipClient) Do(request *http.Request) (*http.Response, error) {
	c.requestedURLs = append(c.requestedURLs, request.URL.String())
	c.requestDeadlines = append(c.requestDeadlines, deadlineOf(request.Context()))

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

func testZip(t *testing.T, filename, jsonContent string) []byte {
	t.Helper()
	zipData, err := createTestZip(filename, []byte(jsonContent))
	if err != nil {
		t.Fatalf("failed to create test zip: %v", err)
	}
	return zipData
}

func testArtifact(id int64, age time.Duration) *github.Artifact {
	return &github.Artifact{
		ID:        github.Ptr(id),
		Name:      github.Ptr("test-artifact"),
		CreatedAt: &github.Timestamp{Time: time.Now().Add(-age)},
	}
}

const loadedStateJSON = `{"version":7,"message":"loaded"}`

func TestFetchLatestArtifactByName(t *testing.T) {
	noResponse := githubCallResult{err: errors.New("connection reset")}
	serverError := githubCallResult{status: 500, err: errors.New("server error")}

	tests := []struct {
		name                  string
		artifacts             []*github.Artifact
		jsonFilePath          string
		zipFilename           string
		zipJSON               string
		zipData               []byte // replaces the whole zip
		listResults           []githubCallResult
		downloadResults       []githubCallResult
		zipResults            []zipDownloadResult
		expectedError         string
		expectNoArtifactFound bool
		expectedArtifactID    int64
		expectedListCalls     int
		expectedDownloadCalls int
		expectedZipDownloads  int
		expectedWaits         []time.Duration
		expectedRetryLog      string
	}{
		{
			name:                  "state loads on the first attempt",
			expectedListCalls:     1,
			expectedDownloadCalls: 1,
			expectedZipDownloads:  1,
		},
		{
			name: "newest of several artifacts is downloaded",
			artifacts: []*github.Artifact{
				testArtifact(123, 2*time.Hour), testArtifact(456, time.Hour), testArtifact(789, 3*time.Hour),
			},
			expectedArtifactID:    456,
			expectedListCalls:     1,
			expectedDownloadCalls: 1,
			expectedZipDownloads:  1,
		},
		{
			name:                  "directory part of the JSON file path is ignored",
			jsonFilePath:          "/tmp/state.json",
			expectedListCalls:     1,
			expectedDownloadCalls: 1,
			expectedZipDownloads:  1,
		},
		{
			name:                  "directory part of the file inside the zip is ignored",
			zipFilename:           "path/to/state.json",
			expectedListCalls:     1,
			expectedDownloadCalls: 1,
			expectedZipDownloads:  1,
		},
		{
			name:                  "JSON file missing from the zip is not retried",
			jsonFilePath:          "missing.json",
			expectedError:         `json file "missing.json" not found inside artifact zip`,
			expectedListCalls:     1,
			expectedDownloadCalls: 1,
			expectedZipDownloads:  1,
		},
		{
			name:                  "JSON not matching the target type is not retried",
			zipJSON:               `{"version":"one"}`,
			expectedError:         `decode json "state.json": json: cannot unmarshal string into Go struct field testState.version of type int`,
			expectedListCalls:     1,
			expectedDownloadCalls: 1,
			expectedZipDownloads:  1,
		},
		{
			name:                  "zip that does not open is not retried",
			zipData:               []byte("not a zip"),
			expectedError:         "open zip: zip: not a valid zip file",
			expectedListCalls:     1,
			expectedDownloadCalls: 1,
			expectedZipDownloads:  1,
		},
		{
			name:                  "no artifact found is not retried",
			artifacts:             []*github.Artifact{},
			expectedError:         `no artifacts found with name "test-artifact"`,
			expectNoArtifactFound: true,
			expectedListCalls:     1,
		},
		{
			name:                  "artifact list server error then success",
			listResults:           []githubCallResult{serverError},
			expectedListCalls:     2,
			expectedDownloadCalls: 1,
			expectedZipDownloads:  1,
			expectedWaits:         []time.Duration{2 * time.Second},
			expectedRetryLog:      "artifact list attempt 1 failed, retrying in 2s: failed to list artifacts: server error status=500 Internal Server Error\n",
		},
		{
			name:                  "artifact list body cut off after a 200 then success",
			listResults:           []githubCallResult{{status: 200, err: context.DeadlineExceeded}},
			expectedListCalls:     2,
			expectedDownloadCalls: 1,
			expectedZipDownloads:  1,
			expectedWaits:         []time.Duration{2 * time.Second},
		},
		{
			name:              "artifact list without a response fails after 3 attempts",
			listResults:       []githubCallResult{noResponse, noResponse, noResponse, noResponse},
			expectedError:     "failed to list artifacts: connection reset",
			expectedListCalls: 3,
			expectedWaits:     []time.Duration{2 * time.Second, 5 * time.Second},
			expectedRetryLog:  "artifact list attempt 2 failed, retrying in 5s: failed to list artifacts: connection reset",
		},
		{
			name:              "artifact list client error just below the server errors is not retried",
			listResults:       []githubCallResult{{status: 499, err: errors.New("client closed request")}},
			expectedError:     "failed to list artifacts: client closed request status=499",
			expectedListCalls: 1,
		},
		{
			name:              "artifact list rate limit 403 is not retried",
			listResults:       []githubCallResult{{status: 403, err: errors.New("API rate limit exceeded")}},
			expectedError:     "failed to list artifacts: API rate limit exceeded status=403 Forbidden",
			expectedListCalls: 1,
		},
		{
			name:              "artifact list 429 is not retried",
			listResults:       []githubCallResult{{status: 429, err: errors.New("too many requests")}},
			expectedError:     "failed to list artifacts: too many requests status=429 Too Many Requests",
			expectedListCalls: 1,
		},
		{
			name:                  "download URL server error then success",
			downloadResults:       []githubCallResult{serverError},
			expectedListCalls:     1,
			expectedDownloadCalls: 2,
			expectedZipDownloads:  1,
			expectedWaits:         []time.Duration{2 * time.Second},
			expectedRetryLog:      "artifact download attempt 1 failed, retrying in 2s: get artifact download URL: server error\n",
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
			expectedError:         "get artifact download URL: unexpected status code: 410 Gone",
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
			expectedRetryLog:      "artifact download attempt 1 failed, retrying in 2s: download artifact zip: connection reset\n",
		},
		{
			name:                  "zip download server error then success",
			zipResults:            []zipDownloadResult{{status: 500}},
			expectedListCalls:     1,
			expectedDownloadCalls: 2,
			expectedZipDownloads:  2,
			expectedWaits:         []time.Duration{2 * time.Second},
			expectedRetryLog:      "artifact download attempt 1 failed, retrying in 2s: unexpected status code 500 when downloading artifact\n",
		},
		{
			name:                  "zip download failing to read the body then success",
			zipResults:            []zipDownloadResult{{status: 200, bodyReadError: errors.New("unexpected EOF")}},
			expectedListCalls:     1,
			expectedDownloadCalls: 2,
			expectedZipDownloads:  2,
			expectedWaits:         []time.Duration{2 * time.Second},
			expectedRetryLog:      "artifact download attempt 1 failed, retrying in 2s: read artifact zip: unexpected EOF\n",
		},
		{
			name:                  "zip download server errors fail after 3 attempts",
			zipResults:            []zipDownloadResult{{status: 503}, {status: 503}, {status: 503}, {status: 503}},
			expectedError:         "unexpected status code 503 when downloading artifact",
			expectedListCalls:     1,
			expectedDownloadCalls: 3,
			expectedZipDownloads:  3,
			expectedWaits:         []time.Duration{2 * time.Second, 5 * time.Second},
		},
		{
			name:                  "zip download client error just below the server errors is not retried",
			zipResults:            []zipDownloadResult{{status: 499}},
			expectedError:         "unexpected status code 499 when downloading artifact",
			expectedListCalls:     1,
			expectedDownloadCalls: 1,
			expectedZipDownloads:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := skipAndRecordRetryWaits(t)
			logOutput := captureLogOutput(t)
			artifacts := tt.artifacts
			if artifacts == nil {
				artifacts = []*github.Artifact{testArtifact(123, time.Hour)}
			}
			actions := &scriptedActionsService{
				listResults:     tt.listResults,
				downloadResults: tt.downloadResults,
				artifacts:       artifacts,
			}
			zipData := tt.zipData
			if zipData == nil {
				zipData = testZip(t, cmp.Or(tt.zipFilename, "state.json"), cmp.Or(tt.zipJSON, loadedStateJSON))
			}
			zipClient := &scriptedZipClient{t: t, results: tt.zipResults, zipData: zipData}

			var loaded testState
			err := NewClient(zipClient, actions, nil).FetchLatestArtifactByName(
				context.Background(), "test-owner", "test-repo", "test-artifact",
				cmp.Or(tt.jsonFilePath, "state.json"), &loaded,
			)

			assertEqualStrings(t, "error", errorText(err), tt.expectedError)
			if errors.Is(err, ErrNoArtifactFound) != tt.expectNoArtifactFound {
				t.Errorf("expected errors.Is(err, ErrNoArtifactFound) to be %v, got %v", tt.expectNoArtifactFound, err)
			}
			if tt.expectedError == "" && loaded != (testState{Version: 7, Message: "loaded"}) {
				t.Errorf("expected the state to load, got %+v", loaded)
			}
			if actions.listCalls != tt.expectedListCalls {
				t.Errorf("expected %d artifact list calls, got %d", tt.expectedListCalls, actions.listCalls)
			}
			if actions.downloadCalls != tt.expectedDownloadCalls {
				t.Errorf("expected %d download URL calls, got %d", tt.expectedDownloadCalls, actions.downloadCalls)
			}
			expectedArtifactID := cmp.Or(tt.expectedArtifactID, 123)
			for _, artifactID := range actions.downloadedArtifactIDs {
				if artifactID != expectedArtifactID {
					t.Errorf("expected artifact %d to be downloaded, got %d", expectedArtifactID, artifactID)
				}
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

func TestFetchLatestArtifactByNameGivesEachCallA15sAttemptDeadline(t *testing.T) {
	actions := &scriptedActionsService{artifacts: []*github.Artifact{testArtifact(123, time.Hour)}}
	zipClient := &scriptedZipClient{t: t, zipData: testZip(t, "state.json", loadedStateJSON)}

	started := time.Now()
	var loaded testState
	err := NewClient(zipClient, actions, nil).FetchLatestArtifactByName(
		context.Background(), "test-owner", "test-repo", "test-artifact", "state.json", &loaded,
	)
	finished := time.Now()

	if err != nil {
		t.Fatalf("expected the state to load, got %v", err)
	}
	deadlines := slices.Concat(actions.listCallDeadlines, actions.downloadCallDeadlines, zipClient.requestDeadlines)
	if len(deadlines) != 3 {
		t.Fatalf("expected one list, one download URL and one zip call, got %d deadlines", len(deadlines))
	}
	for index, deadline := range deadlines {
		if deadline.Before(started.Add(15*time.Second)) || deadline.After(finished.Add(15*time.Second)) {
			t.Errorf("call %d: expected a deadline 15s after its attempt started, got %v from %v", index+1, deadline.Sub(started), started)
		}
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
			skipAndRecordRetryWaits(t)
			withAttemptTimeout(t, 20*time.Millisecond)
			actions := &scriptedActionsService{artifacts: []*github.Artifact{testArtifact(123, time.Hour)}}
			zipClient := &scriptedZipClient{
				t:       t,
				results: tt.zipResults,
				zipData: testZip(t, "state.json", loadedStateJSON),
			}

			var loaded testState
			err := NewClient(zipClient, actions, nil).FetchLatestArtifactByName(
				context.Background(), "test-owner", "test-repo", "test-artifact", "state.json", &loaded,
			)

			if err != nil || loaded.Version != 7 {
				t.Fatalf("expected the second attempt to load the state, got %+v and %v", loaded, err)
			}
			if len(zipClient.requestedURLs) != 2 {
				t.Errorf("expected 2 zip requests, got %d", len(zipClient.requestedURLs))
			}
		})
	}
}
