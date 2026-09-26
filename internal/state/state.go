package state

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/slackclient"
	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/prview"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

const CurrentSchemaVersion = 1

type State struct {
	SchemaVersion                 int                     `json:"schemaVersion"`
	MessagePostedAt               time.Time               `json:"messagePostedAt"`
	MessageRef                    SlackRef                `json:"messageRef"`
	PullRequests                  []models.PullRequestRef `json:"pullRequests"`
	LastWrittenCanvasMarkdownHash string                  `json:"canvasContentHash"`
	LastWrittenMessage            LastWrittenMessage      `json:"lastWrittenMessage"`
}

// Falls back to the keys older releases wrote. See state.spec.md § Oddities.
func (s *State) UnmarshalJSON(data []byte) error {
	type stateWithDefaultDecoding State
	var decoded struct {
		stateWithDefaultDecoding
		LegacyMessagePostedAt time.Time `json:"createdAt"`
		LegacyMessageRef      SlackRef  `json:"slackMessage"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*s = State(decoded.stateWithDefaultDecoding)
	if s.MessagePostedAt.IsZero() {
		s.MessagePostedAt = decoded.LegacyMessagePostedAt
	}
	if s.MessageRef == (SlackRef{}) {
		s.MessageRef = decoded.LegacyMessageRef
	}
	return nil
}

// See state.spec.md.
type LastWrittenMessage struct {
	// Without omitempty, nil blocks save as JSON null, which loads back as non-empty blocks
	Blocks      json.RawMessage `json:"blocks,omitempty"`
	SummaryText string          `json:"summaryText"`
	GeneratedAt time.Time       `json:"generatedAt"`
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

func PRToPullRequestRef(pr prview.PR) models.PullRequestRef {
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
// SchemaVersion and MessagePostedAt.
func NewPostState(
	prViews []prview.PR,
	messageInfo slackclient.SentMessageInfo,
	summaryText string,
	generatedAt time.Time,
) State {
	return State{
		SchemaVersion:   CurrentSchemaVersion,
		MessagePostedAt: time.Now(),
		MessageRef: SlackRef{
			ChannelID: messageInfo.ChannelID,
			MessageTS: messageInfo.Timestamp,
		},
		PullRequests:       utilities.Map(prViews, PRToPullRequestRef),
		LastWrittenMessage: newLastWrittenMessage(messageInfo, summaryText, generatedAt),
	}
}

func WithLastWrittenMessage(
	loadedState State,
	messageInfo slackclient.SentMessageInfo,
	summaryText string,
	generatedAt time.Time,
) State {
	loadedState.LastWrittenMessage = newLastWrittenMessage(messageInfo, summaryText, generatedAt)
	return loadedState
}

func newLastWrittenMessage(
	messageInfo slackclient.SentMessageInfo, summaryText string, generatedAt time.Time,
) LastWrittenMessage {
	return LastWrittenMessage{
		Blocks:      messageInfo.BlocksAsSent,
		SummaryText: summaryText,
		GeneratedAt: generatedAt,
	}
}

func SaveSentSlackBlocksToFile(
	filePath string,
	sentBlocks json.RawMessage,
) error {
	var indentedBlocks bytes.Buffer
	if err := json.Indent(&indentedBlocks, sentBlocks, "", "  "); err != nil {
		return fmt.Errorf("failed to indent sent blocks: %w", err)
	}

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	if err := os.WriteFile(filePath, indentedBlocks.Bytes(), 0644); err != nil {
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
