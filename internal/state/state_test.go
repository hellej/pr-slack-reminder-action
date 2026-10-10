package state

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
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

// A regular file as a parent fails MkdirAll, also for root.
func pathBelowARegularFile(t *testing.T) string {
	t.Helper()
	regularFile := filepath.Join(t.TempDir(), "regular-file")
	if err := os.WriteFile(regularFile, nil, 0644); err != nil {
		t.Fatalf("Failed to create regular file: %v", err)
	}
	return filepath.Join(regularFile, "nested", "file.json")
}

func createTestState() State {
	return State{
		SchemaVersion:   CurrentSchemaVersion,
		MessagePostedAt: time.Now().UTC(),
		MessageRef: SlackRef{
			ChannelID: "C123456789",
			MessageTS: "1729123456.123456",
		},
		TrackedPRs: []models.PullRequestRef{
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
		TrackedPRs: []models.PullRequestRef{
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

	if len(loadedState.TrackedPRs) != len(originalState.TrackedPRs) {
		t.Errorf("TrackedPRs length mismatch: got %d, want %d", len(loadedState.TrackedPRs), len(originalState.TrackedPRs))
	}

	for i, pr := range loadedState.TrackedPRs {
		original := originalState.TrackedPRs[i]
		if pr.Repository.Owner != original.Repository.Owner || pr.Repository.Name != original.Repository.Name || pr.Number != original.Number {
			t.Errorf("TrackedPRs[%d] mismatch: got %+v, want %+v", i, pr, original)
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
	var syntaxError *json.SyntaxError
	var typeError *json.UnmarshalTypeError
	tests := []struct {
		name          string
		content       string
		expectedError any
	}{
		{name: "malformed JSON", content: "{ invalid json content", expectedError: &syntaxError},
		{name: "field of the wrong type", content: `{"pullRequests": "not a list"}`, expectedError: &typeError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			invalidJSONPath := filepath.Join(t.TempDir(), "invalid.json")
			if err := os.WriteFile(invalidJSONPath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("Failed to create invalid JSON file: %v", err)
			}

			_, err := LoadFromFile(invalidJSONPath)
			if !errors.As(err, tt.expectedError) {
				t.Errorf("Expected error of type %T, got: %v", tt.expectedError, err)
			}
		})
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

func TestSaveFailures(t *testing.T) {
	slackBlocksJSON := json.RawMessage(`[{"type":"rich_text","block_id":"test"}]`)
	saveState := func(filePath string) error { return Save(filePath, createTestState()) }
	saveSentBlocks := func(filePath string) error { return SaveSentSlackBlocksToFile(filePath, slackBlocksJSON) }

	tests := []struct {
		name          string
		save          func(filePath string) error
		filePath      func(t *testing.T) string
		expectedError string
	}{
		{
			name:          "state directory cannot be created",
			save:          saveState,
			filePath:      pathBelowARegularFile,
			expectedError: "failed to create directory",
		},
		{
			name:          "sent blocks directory cannot be created",
			save:          saveSentBlocks,
			filePath:      pathBelowARegularFile,
			expectedError: "failed to create directory",
		},
		{
			name: "state with malformed sent blocks",
			save: func(filePath string) error {
				stateWithMalformedBlocks := createTestState()
				stateWithMalformedBlocks.LastWrittenMessage.Blocks = json.RawMessage(`[{"type":`)
				return Save(filePath, stateWithMalformedBlocks)
			},
			filePath:      func(t *testing.T) string { return filepath.Join(t.TempDir(), "state.json") },
			expectedError: "failed to marshal state",
		},
		{
			name:          "state file path is a directory",
			save:          saveState,
			filePath:      func(t *testing.T) string { return t.TempDir() },
			expectedError: "failed to write state file",
		},
		{
			name:          "sent blocks file path is a directory",
			save:          saveSentBlocks,
			filePath:      func(t *testing.T) string { return t.TempDir() },
			expectedError: "failed to write sent blocks file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.save(tt.filePath(t))
			if err == nil {
				t.Fatal("Expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.expectedError) {
				t.Errorf("Expected error containing %q, got: %v", tt.expectedError, err)
			}
		})
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
		TrackedPRs: []models.PullRequestRef{
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

	if len(loadedState.TrackedPRs) != len(expectedState.TrackedPRs) {
		t.Errorf("TrackedPRs length mismatch: got %d, want %d", len(loadedState.TrackedPRs), len(expectedState.TrackedPRs))
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

func TestStateDecodesTheLegacyKeysWhenTheNewOnesAreAbsentOrEmpty(t *testing.T) {
	legacyPR := models.PullRequestRef{Repository: models.Repository{Owner: "o", Name: "legacy"}, Number: 7}
	trackedPR := models.PullRequestRef{Repository: models.Repository{Owner: "o", Name: "tracked"}, Number: 8}
	tests := []struct {
		name                    string
		stateJSON               string
		expectedMessagePostedAt time.Time
		expectedMessageRef      SlackRef
		expectedTrackedPRs      []models.PullRequestRef
	}{
		{
			name: "legacy keys only",
			stateJSON: `{"schemaVersion":1,"createdAt":"2026-09-01T09:00:00Z",` +
				`"slackMessage":{"channelId":"C-LEGACY","messageTs":"1788253200.000100"},` +
				`"pullRequests":[{"Repository":{"Owner":"o","Name":"legacy"},"Number":7}]}`,
			expectedMessagePostedAt: time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC),
			expectedMessageRef:      SlackRef{ChannelID: "C-LEGACY", MessageTS: "1788253200.000100"},
			expectedTrackedPRs:      []models.PullRequestRef{legacyPR},
		},
		{
			name: "new keys win over legacy ones",
			stateJSON: `{"schemaVersion":1,"createdAt":"2026-09-01T09:00:00Z",` +
				`"slackMessage":{"channelId":"C-LEGACY","messageTs":"1788253200.000100"},` +
				`"pullRequests":[{"Repository":{"Owner":"o","Name":"legacy"},"Number":7}],` +
				`"messagePostedAt":"2026-09-02T09:00:00Z",` +
				`"messageRef":{"channelId":"C-NEW","messageTs":"1788339600.000200"},` +
				`"trackedPRs":[{"Repository":{"Owner":"o","Name":"tracked"},"Number":8}]}`,
			expectedMessagePostedAt: time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC),
			expectedMessageRef:      SlackRef{ChannelID: "C-NEW", MessageTS: "1788339600.000200"},
			expectedTrackedPRs:      []models.PullRequestRef{trackedPR},
		},
		{
			name: "empty new keys fall back to legacy ones",
			stateJSON: `{"schemaVersion":1,"createdAt":"2026-09-01T09:00:00Z",` +
				`"slackMessage":{"channelId":"C-LEGACY","messageTs":"1788253200.000100"},` +
				`"pullRequests":[{"Repository":{"Owner":"o","Name":"legacy"},"Number":7}],` +
				`"messagePostedAt":"0001-01-01T00:00:00Z",` +
				`"messageRef":{"channelId":"","messageTs":""},` +
				`"trackedPRs":[]}`,
			expectedMessagePostedAt: time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC),
			expectedMessageRef:      SlackRef{ChannelID: "C-LEGACY", MessageTS: "1788253200.000100"},
			expectedTrackedPRs:      []models.PullRequestRef{legacyPR},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var decoded State
			if err := json.Unmarshal([]byte(tt.stateJSON), &decoded); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}
			if !decoded.MessagePostedAt.Equal(tt.expectedMessagePostedAt) {
				t.Errorf("MessagePostedAt: got %v, want %v", decoded.MessagePostedAt, tt.expectedMessagePostedAt)
			}
			if decoded.MessageRef != tt.expectedMessageRef {
				t.Errorf("MessageRef: got %+v, want %+v", decoded.MessageRef, tt.expectedMessageRef)
			}
			if !slices.Equal(decoded.TrackedPRs, tt.expectedTrackedPRs) {
				t.Errorf("TrackedPRs: got %+v, want %+v", decoded.TrackedPRs, tt.expectedTrackedPRs)
			}
			if decoded.SchemaVersion != 1 {
				t.Errorf("SchemaVersion: got %d, want 1", decoded.SchemaVersion)
			}
		})
	}
}

func TestWithTrackedPRsAddedAppendsNewlyListedRefsAfterTheLoadedOnes(t *testing.T) {
	repoA := models.Repository{Owner: "o", Name: "repo-a"}
	repoB := models.Repository{Owner: "o", Name: "repo-b"}
	loadedRefsWithSpareCapacity := make([]models.PullRequestRef, 3, 10)
	copy(loadedRefsWithSpareCapacity, []models.PullRequestRef{
		{Repository: repoA, Number: 5},
		{Repository: repoA, Number: 2},
		{Repository: repoB, Number: 3},
	})
	loadedState := createTestState()
	loadedState.TrackedPRs = loadedRefsWithSpareCapacity
	listedPRViews := []prview.PR{
		createTestPR(9, "o", "repo-a"),
		createTestPR(2, "o", "repo-a"),
		createTestPR(2, "o", "repo-b"),
		createTestPR(1, "o", "repo-a"),
	}

	result := WithTrackedPRsAdded(loadedState, listedPRViews)

	expectedTrackedPRs := []models.PullRequestRef{
		{Repository: repoA, Number: 5},
		{Repository: repoA, Number: 2},
		{Repository: repoB, Number: 3},
		{Repository: repoA, Number: 9},
		{Repository: repoB, Number: 2},
		{Repository: repoA, Number: 1},
	}
	if !slices.Equal(result.TrackedPRs, expectedTrackedPRs) {
		t.Errorf("TrackedPRs: got %+v, want %+v", result.TrackedPRs, expectedTrackedPRs)
	}
	if result.MessageRef != loadedState.MessageRef {
		t.Errorf("MessageRef: got %+v, want the loaded %+v", result.MessageRef, loadedState.MessageRef)
	}
	if loadedState.TrackedPRs[:cap(loadedState.TrackedPRs)][3] != (models.PullRequestRef{}) {
		t.Errorf("Wrote into the loaded state's spare capacity: %+v", loadedState.TrackedPRs[:cap(loadedState.TrackedPRs)])
	}
}
