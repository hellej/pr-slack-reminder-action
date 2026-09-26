package state

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/prview"
)

func LoadFromFile(filePath string) (*State, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	return &state, nil
}

func setupReadOnlyDir(t *testing.T) string {
	t.Helper()
	if os.Getuid() == 0 {
		t.Skip("Test requires non-root user to fail file write")
	}

	tempDir := t.TempDir()
	readOnlyDir := filepath.Join(tempDir, "readonly")
	if err := os.Mkdir(readOnlyDir, 0555); err != nil {
		t.Fatalf("Failed to create read-only directory: %v", err)
	}
	return readOnlyDir
}

func createTestState() State {
	return State{
		SchemaVersion:   CurrentSchemaVersion,
		MessagePostedAt: time.Now().UTC(),
		MessageRef: SlackRef{
			ChannelID: "C123456789",
			MessageTS: "1729123456.123456",
		},
		PullRequests: []models.PullRequestRef{
			{Repository: models.Repository{Owner: "owner1", Name: "repo1"}, Number: 1},
		},
	}
}

func createTestPR(number int, owner, repo string) prview.PR {
	return prview.PR{
		PR: &githubclient.PR{
			PullRequest: &githubclient.PullRequest{Number: number},
			Repository:  models.Repository{Owner: owner, Name: repo},
		},
	}
}

func TestStateSaveAndLoadRoundTrip(t *testing.T) {
	tempDir := t.TempDir()
	statePath := filepath.Join(tempDir, "state.json")

	originalState := State{
		SchemaVersion:   CurrentSchemaVersion,
		MessagePostedAt: time.Now().UTC(),
		MessageRef: SlackRef{
			ChannelID: "C123456789",
			MessageTS: "1729123456.123456",
		},
		PullRequests: []models.PullRequestRef{
			{Repository: models.Repository{Owner: "owner1", Name: "repo1"}, Number: 1},
			{Repository: models.Repository{Owner: "owner1", Name: "repo1"}, Number: 2},
			{Repository: models.Repository{Owner: "owner2", Name: "repo2"}, Number: 5},
		},
	}

	err := Save(statePath, originalState)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loadedState, err := LoadFromFile(statePath)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	if loadedState.SchemaVersion != originalState.SchemaVersion {
		t.Errorf("SchemaVersion mismatch: got %d, want %d", loadedState.SchemaVersion, originalState.SchemaVersion)
	}

	if !loadedState.MessagePostedAt.Equal(originalState.MessagePostedAt) {
		t.Errorf("MessagePostedAt mismatch: got %v, want %v", loadedState.MessagePostedAt, originalState.MessagePostedAt)
	}

	if loadedState.MessageRef.ChannelID != originalState.MessageRef.ChannelID {
		t.Errorf("MessageRef.ChannelID mismatch: got %s, want %s", loadedState.MessageRef.ChannelID, originalState.MessageRef.ChannelID)
	}

	if loadedState.MessageRef.MessageTS != originalState.MessageRef.MessageTS {
		t.Errorf("MessageRef.MessageTS mismatch: got %s, want %s", loadedState.MessageRef.MessageTS, originalState.MessageRef.MessageTS)
	}

	if len(loadedState.PullRequests) != len(originalState.PullRequests) {
		t.Errorf("PullRequests length mismatch: got %d, want %d", len(loadedState.PullRequests), len(originalState.PullRequests))
	}

	for i, pr := range loadedState.PullRequests {
		original := originalState.PullRequests[i]
		if pr.Repository.Owner != original.Repository.Owner || pr.Repository.Name != original.Repository.Name || pr.Number != original.Number {
			t.Errorf("PullRequest[%d] mismatch: got %+v, want %+v", i, pr, original)
		}
	}
}

func TestLoadFileNotFound(t *testing.T) {
	nonExistentPath := "/tmp/non-existent-state.json"

	_, err := LoadFromFile(nonExistentPath)
	if err == nil {
		t.Fatal("Expected error when loading non-existent file, got nil")
	}

	if !os.IsNotExist(err) {
		t.Errorf("Expected file not found error, got: %v", err)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	tempDir := t.TempDir()
	invalidJSONPath := filepath.Join(tempDir, "invalid.json")

	err := os.WriteFile(invalidJSONPath, []byte("{ invalid json content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create invalid JSON file: %v", err)
	}

	_, err = LoadFromFile(invalidJSONPath)
	if err == nil {
		t.Fatal("Expected error when loading invalid JSON, got nil")
	}

	var jsonErr *json.SyntaxError
	if !errors.As(err, &jsonErr) {
		t.Errorf("Expected JSON syntax error, got: %v", err)
	}
}

// Empty blocks mean the info did not come from a send. An empty record would hide that.
func TestSaveSentSlackBlocksToFileRejectsEmptyBlocks(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "empty-blocks.json")

	err := SaveSentSlackBlocksToFile(filePath, nil)
	if err == nil {
		t.Fatal("Expected error when saving empty blocks, got nil")
	}
	if !strings.Contains(err.Error(), "failed to indent sent blocks") {
		t.Errorf("Expected JSON indent error, got: %v", err)
	}
	if _, statErr := os.Stat(filePath); !os.IsNotExist(statErr) {
		t.Errorf("Expected no file written, stat returned: %v", statErr)
	}
}

func TestSaveSentSlackBlocksToFileInvalidJSON(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "invalid-blocks.json")

	err := SaveSentSlackBlocksToFile(filePath, json.RawMessage(`[{"type":"rich_text",`))
	if err == nil {
		t.Fatal("Expected error when saving invalid JSON, got nil")
	}

	if !strings.Contains(err.Error(), "failed to indent sent blocks") {
		t.Errorf("Expected JSON indent error, got: %v", err)
	}
}

func TestSaveDirectoryCreationFailure(t *testing.T) {
	readOnlyDir := setupReadOnlyDir(t)
	statePath := filepath.Join(readOnlyDir, "nested", "state.json")
	state := createTestState()

	err := Save(statePath, state)
	if err == nil {
		t.Fatal("Expected error when creating directory in read-only parent, got nil")
	}

	if !strings.Contains(err.Error(), "failed to create directory") {
		t.Errorf("Expected directory creation error, got: %v", err)
	}
}

func TestSaveSentSlackBlocksToFileDirectoryCreationFailure(t *testing.T) {
	readOnlyDir := setupReadOnlyDir(t)
	filePath := filepath.Join(readOnlyDir, "nested", "blocks.json")
	slackBlocksJSON := json.RawMessage(`[{"type":"rich_text","block_id":"test"}]`)

	err := SaveSentSlackBlocksToFile(filePath, slackBlocksJSON)
	if err == nil {
		t.Fatal("Expected error when creating directory in read-only parent, got nil")
	}

	if !strings.Contains(err.Error(), "failed to create directory") {
		t.Errorf("Expected directory creation error, got: %v", err)
	}
}

func TestSaveFileWriteFailure(t *testing.T) {
	readOnlyDir := setupReadOnlyDir(t)
	statePath := filepath.Join(readOnlyDir, "state.json")
	state := createTestState()

	err := Save(statePath, state)
	if err == nil {
		t.Fatal("Expected error when writing to read-only directory, got nil")
	}

	if !strings.Contains(err.Error(), "failed to write state file") {
		t.Errorf("Expected file write error, got: %v", err)
	}
}

func TestSaveSentSlackBlocksToFileFileWriteFailure(t *testing.T) {
	readOnlyDir := setupReadOnlyDir(t)
	filePath := filepath.Join(readOnlyDir, "blocks.json")
	slackBlocksJSON := json.RawMessage(`[{"type":"rich_text","block_id":"test"}]`)

	err := SaveSentSlackBlocksToFile(filePath, slackBlocksJSON)
	if err == nil {
		t.Fatal("Expected error when writing to read-only directory, got nil")
	}

	if !strings.Contains(err.Error(), "failed to write sent blocks file") {
		t.Errorf("Expected file write error, got: %v", err)
	}
}

type mockStateArtifactFetcher struct {
	fetchError error
	state      *State
}

func (m *mockStateArtifactFetcher) FetchLatestArtifactByName(
	ctx context.Context,
	owner, repo, artifactName, jsonFilePath string,
	target any,
) error {
	if m.fetchError != nil {
		return m.fetchError
	}
	if m.state != nil {
		statePtr, ok := target.(*State)
		if ok {
			*statePtr = *m.state
		}
	}
	return nil
}

func TestLoadSuccessful(t *testing.T) {
	expectedState := &State{
		SchemaVersion:   CurrentSchemaVersion,
		MessagePostedAt: time.Now().UTC(),
		MessageRef: SlackRef{
			ChannelID: "C123456789",
			MessageTS: "1729123456.123456",
		},
		PullRequests: []models.PullRequestRef{
			{Repository: models.Repository{Owner: "owner1", Name: "repo1"}, Number: 1},
			{Repository: models.Repository{Owner: "owner2", Name: "repo2"}, Number: 42},
		},
	}

	mockFetcher := &mockStateArtifactFetcher{state: expectedState}
	repository := models.Repository{Owner: "owner1", Name: "repo1"}

	loadedState, err := Load(context.Background(), mockFetcher, repository, "test-artifact", "state.json")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loadedState.SchemaVersion != expectedState.SchemaVersion {
		t.Errorf("SchemaVersion mismatch: got %d, want %d", loadedState.SchemaVersion, expectedState.SchemaVersion)
	}

	if loadedState.MessageRef.ChannelID != expectedState.MessageRef.ChannelID {
		t.Errorf("ChannelID mismatch: got %s, want %s", loadedState.MessageRef.ChannelID, expectedState.MessageRef.ChannelID)
	}

	if len(loadedState.PullRequests) != len(expectedState.PullRequests) {
		t.Errorf("PullRequests length mismatch: got %d, want %d", len(loadedState.PullRequests), len(expectedState.PullRequests))
	}
}

func TestLoadFetchError(t *testing.T) {
	expectedError := errors.New("artifact fetch failed")
	mockFetcher := &mockStateArtifactFetcher{fetchError: expectedError}
	repository := models.Repository{Owner: "owner1", Name: "repo1"}

	_, err := Load(context.Background(), mockFetcher, repository, "test-artifact", "state.json")
	if err == nil {
		t.Fatal("Expected error from Load, got nil")
	}

	if !errors.Is(err, expectedError) {
		t.Errorf("Expected error %v, got %v", expectedError, err)
	}
}

func TestPRToPullRequestRef(t *testing.T) {
	pr := createTestPR(123, "test-owner", "test-repo")

	ref := PRToPullRequestRef(pr)

	if ref.Number != 123 {
		t.Errorf("Expected Number to be 123, got %d", ref.Number)
	}

	if ref.Repository.Owner != "test-owner" {
		t.Errorf("Expected Owner to be 'test-owner', got %s", ref.Repository.Owner)
	}

	if ref.Repository.Name != "test-repo" {
		t.Errorf("Expected Name to be 'test-repo', got %s", ref.Repository.Name)
	}
}
