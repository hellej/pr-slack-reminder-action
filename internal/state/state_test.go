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

	"github.com/google/go-github/v78/github"
	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/slackclient"
	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/prparser"
	"github.com/hellej/pr-slack-reminder-action/testhelpers"
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
		SchemaVersion: CurrentSchemaVersion,
		CreatedAt:     time.Now().UTC(),
		SlackMessage: SlackRef{
			ChannelID: "C123456789",
			MessageTS: "1729123456.123456",
		},
		PullRequests: []models.PullRequestRef{
			{Repository: models.NewRepository("owner1", "repo1"), Number: 1},
		},
	}
}

func createTestPR(number int, owner, repo string) prparser.PR {
	return prparser.PR{
		PR: &githubclient.PR{
			PullRequest: &github.PullRequest{Number: testhelpers.AsPointer(number)},
			Repository:  models.NewRepository(owner, repo),
		},
	}
}

func TestStateSaveAndLoadRoundTrip(t *testing.T) {
	tempDir := t.TempDir()
	statePath := filepath.Join(tempDir, "state.json")

	originalState := State{
		SchemaVersion: CurrentSchemaVersion,
		CreatedAt:     time.Now().UTC(),
		SlackMessage: SlackRef{
			ChannelID: "C123456789",
			MessageTS: "1729123456.123456",
		},
		PullRequests: []models.PullRequestRef{
			{Repository: models.NewRepository("owner1", "repo1"), Number: 1},
			{Repository: models.NewRepository("owner1", "repo1"), Number: 2},
			{Repository: models.NewRepository("owner2", "repo2"), Number: 5},
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

	if !loadedState.CreatedAt.Equal(originalState.CreatedAt) {
		t.Errorf("CreatedAt mismatch: got %v, want %v", loadedState.CreatedAt, originalState.CreatedAt)
	}

	if loadedState.SlackMessage.ChannelID != originalState.SlackMessage.ChannelID {
		t.Errorf("SlackMessage.ChannelID mismatch: got %s, want %s", loadedState.SlackMessage.ChannelID, originalState.SlackMessage.ChannelID)
	}

	if loadedState.SlackMessage.MessageTS != originalState.SlackMessage.MessageTS {
		t.Errorf("SlackMessage.MessageTS mismatch: got %s, want %s", loadedState.SlackMessage.MessageTS, originalState.SlackMessage.MessageTS)
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

func TestStateValidateSchemaVersionMismatch(t *testing.T) {
	state := State{
		SchemaVersion: CurrentSchemaVersion + 1, // Wrong version
		CreatedAt:     time.Now().UTC(),
		SlackMessage: SlackRef{
			ChannelID: "C123456789",
			MessageTS: "1729123456.123456",
		},
		PullRequests: []models.PullRequestRef{
			{Repository: models.NewRepository("owner1", "repo1"), Number: 1},
		},
	}

	err := state.Validate()
	if err == nil {
		t.Fatal("Expected validation error for schema version mismatch, got nil")
	}

	expectedMsg := "unsupported schema version"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Expected error message to contain %q, got: %v", expectedMsg, err)
	}
}

func TestStateValidateValidState(t *testing.T) {
	state := State{
		SchemaVersion: CurrentSchemaVersion,
		CreatedAt:     time.Now().UTC(),
		SlackMessage: SlackRef{
			ChannelID: "C123456789",
			MessageTS: "1729123456.123456",
		},
		PullRequests: []models.PullRequestRef{
			{Repository: models.NewRepository("owner1", "repo1"), Number: 1},
		},
	}

	err := state.Validate()
	if err != nil {
		t.Errorf("Expected valid state to pass validation, got error: %v", err)
	}
}

func TestSaveSentSlackBlocksProperJSON(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "sent-blocks.json")

	slackBlocksJSON := []string{
		`{"type":"rich_text","block_id":"pr_list_heading","elements":[{"type":"rich_text_section","elements":[{"type":"text","text":"There are 2 open PRs 🚀","style":{"bold":true}}]}]}`,
		`{"type":"rich_text","block_id":"open_prs","elements":[{"type":"rich_text_list","elements":[{"type":"rich_text_section","elements":[{"type":"link","url":"https://github.com/owner/repo/pull/1","text":"Test PR","style":{"bold":true}}]}],"style":"bullet"}]}`,
	}

	err := SaveSentSlackBlocks(filePath, slackBlocksJSON)
	if err != nil {
		t.Fatalf("SaveSentSlackBlocks failed: %v", err)
	}

	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read saved file: %v", err)
	}

	var savedBlocks []map[string]interface{}
	err = json.Unmarshal(fileContent, &savedBlocks)
	if err != nil {
		t.Fatalf("Saved file contains invalid JSON: %v", err)
	}

	if len(savedBlocks) != 2 {
		t.Errorf("Expected 2 blocks, got %d", len(savedBlocks))
	}

	// Verify the content doesn't contain escaped quotes (common sign of double-encoding)
	contentStr := string(fileContent)
	if strings.Contains(contentStr, `\"type\"`) {
		t.Error("Saved JSON contains escaped quotes, indicating double-encoding")
	}

	if len(savedBlocks) > 0 {
		if savedBlocks[0]["type"] != "rich_text" {
			t.Errorf("Expected first block type to be 'rich_text', got %v", savedBlocks[0]["type"])
		}
		if savedBlocks[0]["block_id"] != "pr_list_heading" {
			t.Errorf("Expected first block_id to be 'pr_list_heading', got %v", savedBlocks[0]["block_id"])
		}
	}
}

func TestSaveSentSlackBlocksEmptySlice(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "empty-blocks.json")

	err := SaveSentSlackBlocks(filePath, []string{})
	if err != nil {
		t.Fatalf("SaveSentSlackBlocks failed with empty slice: %v", err)
	}

	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read saved file: %v", err)
	}

	var savedBlocks []map[string]interface{}
	err = json.Unmarshal(fileContent, &savedBlocks)
	if err != nil {
		t.Fatalf("Saved file contains invalid JSON: %v", err)
	}

	if len(savedBlocks) != 0 {
		t.Errorf("Expected empty array, got %d items", len(savedBlocks))
	}
}

func TestSaveSentSlackBlocksInvalidJSON(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "invalid-blocks.json")

	invalidJSON := []string{
		`{"type":"rich_text",`, // Incomplete JSON
	}

	err := SaveSentSlackBlocks(filePath, invalidJSON)
	if err == nil {
		t.Fatal("Expected error when saving invalid JSON, got nil")
	}

	if !strings.Contains(err.Error(), "failed to parse block") {
		t.Errorf("Expected JSON parse error, got: %v", err)
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

func TestSaveSentSlackBlocksDirectoryCreationFailure(t *testing.T) {
	readOnlyDir := setupReadOnlyDir(t)
	filePath := filepath.Join(readOnlyDir, "nested", "blocks.json")
	slackBlocksJSON := []string{
		`{"type":"rich_text","block_id":"test"}`,
	}

	err := SaveSentSlackBlocks(filePath, slackBlocksJSON)
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

func TestSaveSentSlackBlocksFileWriteFailure(t *testing.T) {
	readOnlyDir := setupReadOnlyDir(t)
	filePath := filepath.Join(readOnlyDir, "blocks.json")
	slackBlocksJSON := []string{
		`{"type":"rich_text","block_id":"test"}`,
	}

	err := SaveSentSlackBlocks(filePath, slackBlocksJSON)
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
		SchemaVersion: CurrentSchemaVersion,
		CreatedAt:     time.Now().UTC(),
		SlackMessage: SlackRef{
			ChannelID: "C123456789",
			MessageTS: "1729123456.123456",
		},
		PullRequests: []models.PullRequestRef{
			{Repository: models.NewRepository("owner1", "repo1"), Number: 1},
			{Repository: models.NewRepository("owner2", "repo2"), Number: 42},
		},
	}

	mockFetcher := &mockStateArtifactFetcher{state: expectedState}
	repository := models.NewRepository("owner1", "repo1")

	loadedState, err := Load(context.Background(), mockFetcher, repository, "test-artifact", "state.json")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loadedState.SchemaVersion != expectedState.SchemaVersion {
		t.Errorf("SchemaVersion mismatch: got %d, want %d", loadedState.SchemaVersion, expectedState.SchemaVersion)
	}

	if loadedState.SlackMessage.ChannelID != expectedState.SlackMessage.ChannelID {
		t.Errorf("ChannelID mismatch: got %s, want %s", loadedState.SlackMessage.ChannelID, expectedState.SlackMessage.ChannelID)
	}

	if len(loadedState.PullRequests) != len(expectedState.PullRequests) {
		t.Errorf("PullRequests length mismatch: got %d, want %d", len(loadedState.PullRequests), len(expectedState.PullRequests))
	}
}

func TestLoadFetchError(t *testing.T) {
	expectedError := errors.New("artifact fetch failed")
	mockFetcher := &mockStateArtifactFetcher{fetchError: expectedError}
	repository := models.NewRepository("owner1", "repo1")

	_, err := Load(context.Background(), mockFetcher, repository, "test-artifact", "state.json")
	if err == nil {
		t.Fatal("Expected error from Load, got nil")
	}

	if !errors.Is(err, expectedError) {
		t.Errorf("Expected error %v, got %v", expectedError, err)
	}
}

func TestSavePostStateSuccessful(t *testing.T) {
	tempDir := t.TempDir()
	statePath := filepath.Join(tempDir, "post-state.json")

	parsedPRs := []prparser.PR{
		createTestPR(1, "owner1", "repo1"),
		createTestPR(42, "owner2", "repo2"),
	}

	messageInfo := slackclient.SentMessageInfo{
		ChannelID: "C123456789",
		Timestamp: "1729123456.123456",
	}

	err := SavePostState(statePath, parsedPRs, messageInfo)
	if err != nil {
		t.Fatalf("SavePostState failed: %v", err)
	}

	loadedState, err := LoadFromFile(statePath)
	if err != nil {
		t.Fatalf("Failed to load saved state: %v", err)
	}

	if loadedState.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("SchemaVersion mismatch: got %d, want %d", loadedState.SchemaVersion, CurrentSchemaVersion)
	}

	if loadedState.SlackMessage.ChannelID != messageInfo.ChannelID {
		t.Errorf("ChannelID mismatch: got %s, want %s", loadedState.SlackMessage.ChannelID, messageInfo.ChannelID)
	}

	if loadedState.SlackMessage.MessageTS != messageInfo.Timestamp {
		t.Errorf("MessageTS mismatch: got %s, want %s", loadedState.SlackMessage.MessageTS, messageInfo.Timestamp)
	}

	if len(loadedState.PullRequests) != 2 {
		t.Errorf("Expected 2 PRs, got %d", len(loadedState.PullRequests))
	}

	if len(loadedState.PullRequests) >= 1 {
		pr := loadedState.PullRequests[0]
		if pr.Number != 1 || pr.Repository.Owner != "owner1" || pr.Repository.Name != "repo1" {
			t.Errorf("PR 0 mismatch: got %+v", pr)
		}
	}

	if len(loadedState.PullRequests) >= 2 {
		pr := loadedState.PullRequests[1]
		if pr.Number != 42 || pr.Repository.Owner != "owner2" || pr.Repository.Name != "repo2" {
			t.Errorf("PR 1 mismatch: got %+v", pr)
		}
	}
}

func TestSavePostStateWriteFailure(t *testing.T) {
	readOnlyDir := setupReadOnlyDir(t)
	statePath := filepath.Join(readOnlyDir, "post-state.json")
	parsedPRs := []prparser.PR{createTestPR(1, "owner1", "repo1")}

	messageInfo := slackclient.SentMessageInfo{
		ChannelID: "C123456789",
		Timestamp: "1729123456.123456",
	}

	err := SavePostState(statePath, parsedPRs, messageInfo)
	if err == nil {
		t.Fatal("Expected error when writing to read-only directory, got nil")
	}

	if !strings.Contains(err.Error(), "failed to save state") {
		t.Errorf("Expected save state error, got: %v", err)
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
