package state

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/slackclient"
	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/prparser"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

const CurrentSchemaVersion = 1

type State struct {
	SchemaVersion int                     `json:"schemaVersion"`
	CreatedAt     time.Time               `json:"createdAt"`
	SlackMessage  SlackRef                `json:"slackMessage"`
	PullRequests  []models.PullRequestRef `json:"pullRequests"`
	// Hash of the markdown last written to the PR tracker canvas, so a run rendering the same
	// content can leave the canvas alone. Empty when no canvas was written.
	CanvasContentHash string `json:"canvasContentHash"`
}

type SlackRef struct {
	ChannelID string `json:"channelId"`
	MessageTS string `json:"messageTs"`
}

type StateArtifactFetcher interface {
	FetchLatestArtifactByName(
		ctx context.Context,
		owner, repo, artifactName, jsonFilePath string,
		target any,
	) error
}

func PRToPullRequestRef(pr prparser.PR) models.PullRequestRef {
	return models.PullRequestRef{
		Repository: pr.Repository,
		Number:     pr.GetNumber(),
	}
}

func Load(
	ctx context.Context,
	reader StateArtifactFetcher,
	repository models.Repository,
	artifactName string,
	stateFilePath string,
) (*State, error) {
	var state State
	if err := reader.FetchLatestArtifactByName(
		ctx,
		repository.Owner, repository.Name,
		artifactName, stateFilePath,
		&state,
	); err != nil {
		return nil, err
	}
	return &state, nil
}

// NewPostState builds the state a "post" run leaves behind. The only place stamping
// SchemaVersion and CreatedAt.
func NewPostState(
	parsedPRs []prparser.PR,
	messageInfo slackclient.SentMessageInfo,
) State {
	return State{
		SchemaVersion: CurrentSchemaVersion,
		CreatedAt:     time.Now(),
		SlackMessage: SlackRef{
			ChannelID: messageInfo.ChannelID,
			MessageTS: messageInfo.Timestamp,
		},
		PullRequests: utilities.Map(parsedPRs, PRToPullRequestRef),
	}
}

func SaveSentSlackBlocksToFile(
	filePath string,
	sentBlocks []string,
) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Parse JSON strings back to raw JSON objects to avoid double-encoding
	var parsedBlocks []json.RawMessage
	for i, blockJSON := range sentBlocks {
		var rawMessage json.RawMessage
		if err := json.Unmarshal([]byte(blockJSON), &rawMessage); err != nil {
			return fmt.Errorf("failed to parse block %d as JSON: %w", i, err)
		}
		parsedBlocks = append(parsedBlocks, rawMessage)
	}

	jsonData, err := json.MarshalIndent(parsedBlocks, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal sent blocks: %w", err)
	}

	if err := os.WriteFile(filePath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write sent blocks file %s: %w", filePath, err)
	}
	log.Printf("Saved sent Slack blocks JSON to %s", filePath)
	return nil
}

func Save(filePath string, state State) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	jsonData, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	if err := os.WriteFile(filePath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write state file %s: %w", filePath, err)
	}
	log.Printf("Saved state to %s with %d PRs", filePath, len(state.PullRequests))
	return nil
}
