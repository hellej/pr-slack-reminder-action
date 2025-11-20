package githubclient_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/google/go-github/v78/github"
	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
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
		return nil, &github.Response{Response: &http.Response{StatusCode: 500}}, m.listError
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
	zipData    []byte
	statusCode int
	err        error
}

func (m *mockHTTPClientWithZip) Get(url string) (*http.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &http.Response{
		StatusCode: m.statusCode,
		Body:       io.NopCloser(bytes.NewReader(m.zipData)),
	}, nil
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
	tests := []struct {
		name          string
		artifactName  string
		jsonFilePath  string
		artifacts     []*github.Artifact
		zipFilename   string
		zipContent    testState
		listError     error
		downloadError error
		httpError     error
		httpStatus    int
		expectedData  testState
		expectError   bool
		errorContains string
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
			name:          "no artifacts found",
			artifactName:  "missing-artifact",
			jsonFilePath:  "state.json",
			artifacts:     []*github.Artifact{},
			expectError:   true,
			errorContains: "no artifacts found with name",
		},
		{
			name:          "list artifacts error",
			artifactName:  "test-artifact",
			jsonFilePath:  "state.json",
			listError:     fmt.Errorf("API error"),
			expectError:   true,
			errorContains: "failed to list artifacts",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var zipData []byte
			var err error

			if tt.zipFilename != "" && !tt.expectError {
				jsonContent, _ := json.Marshal(tt.zipContent)
				zipData, err = createTestZip(tt.zipFilename, jsonContent)
				if err != nil {
					t.Fatalf("Failed to create test zip: %v", err)
				}
			} else if tt.zipFilename != "" && tt.httpStatus == 200 {
				jsonContent, _ := json.Marshal(tt.zipContent)
				zipData, _ = createTestZip(tt.zipFilename, jsonContent)
			}

			mockHTTPClient := &mockHTTPClientWithZip{
				zipData:    zipData,
				statusCode: tt.httpStatus,
				err:        tt.httpError,
			}

			downloadURL, _ := url.Parse("https://example.com/download")
			mockActions := &mockActionsServiceWithArtifacts{
				artifacts:      tt.artifacts,
				downloadURL:    downloadURL,
				listError:      tt.listError,
				downloadError:  tt.downloadError,
				mockHTTPClient: mockHTTPClient,
			}

			mockPRService := &mockPullRequestService{
				mockResponse: &github.Response{Response: &http.Response{StatusCode: 200}},
			}
			mockIssueService := &mockIssueService{
				mockResponse: &github.Response{Response: &http.Response{StatusCode: 200}},
			}

			client := githubclient.NewClient(mockHTTPClient, mockPRService, mockIssueService, mockActions)

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
