package state

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/slackclient"
	"github.com/hellej/pr-slack-reminder-action/internal/mergestats"
	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/prview"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

const CurrentSchemaVersion = 1

type State struct {
	SchemaVersion                 int                     `json:"schemaVersion"`
	MessagePostedAt               time.Time               `json:"messagePostedAt"`
	MessageRef                    SlackRef                `json:"messageRef"`
	TrackedPRs                    []models.PullRequestRef `json:"trackedPRs"`
	LastWrittenCanvasMarkdownHash string                  `json:"canvasContentHash"`
	LastWrittenMessage            LastWrittenMessage      `json:"lastWrittenMessage"`
	MergedPRCache                 []githubclient.PR       `json:"mergedPRCache,omitempty"`
	MergeStats                    *mergestats.MergeStats  `json:"mergeStats,omitempty"`
	RecentMergeStats              RecentMergeStats        `json:"recentMergeStats,omitempty"`
}

// Falls back to the keys older releases wrote. See state.spec.md § Oddities.
func (s *State) UnmarshalJSON(data []byte) error {
	type stateWithDefaultDecoding State
	var decoded struct {
		stateWithDefaultDecoding
		LegacyMessagePostedAt time.Time               `json:"createdAt"`
		LegacyMessageRef      SlackRef                `json:"slackMessage"`
		LegacyTrackedPRs      []models.PullRequestRef `json:"pullRequests"`
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
	if len(s.TrackedPRs) == 0 {
		s.TrackedPRs = decoded.LegacyTrackedPRs
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
	mergeStats *mergestats.MergeStats,
) State {
	return State{
		SchemaVersion:   CurrentSchemaVersion,
		MessagePostedAt: time.Now(),
		MessageRef: SlackRef{
			ChannelID: messageInfo.ChannelID,
			MessageTS: messageInfo.Timestamp,
		},
		TrackedPRs:         utilities.Map(prViews, PRToPullRequestRef),
		LastWrittenMessage: newLastWrittenMessage(messageInfo, summaryText, generatedAt),
		MergeStats:         mergeStats,
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

func WithTrackedPRsAdded(loadedState State, listedPRViews []prview.PR) State {
	newlyListedRefs := utilities.Filter(
		utilities.Map(listedPRViews, PRToPullRequestRef),
		func(ref models.PullRequestRef) bool { return !slices.Contains(loadedState.TrackedPRs, ref) },
	)
	loadedState.TrackedPRs = slices.Concat(loadedState.TrackedPRs, newlyListedRefs)
	return loadedState
}

// A PR whose enrichment failed stays out, so the next run retries it.
func WithMergedPRCache(s State, prs []githubclient.PR) State {
	cacheablePRs := utilities.Filter(prs, func(pr githubclient.PR) bool {
		return pr.GetMerged() && pr.HasCompleteReviewerInfo
	})
	s.MergedPRCache = utilities.UniqueFunc(cacheablePRs, isSamePullRequest)
	return s
}

const daysInAWeek = 7

// Oldest first. See state.spec.md § Behaviour.
type RecentMergeStats []mergestats.MergeStats

func (r RecentMergeStats) WithAdded(newMergeStats *mergestats.MergeStats, asOf time.Time) RecentMergeStats {
	oldestKeptDate := utcDateAWeekBefore(asOf)
	recentMergeStats := utilities.Filter(r, func(entry mergestats.MergeStats) bool {
		return !utcDate(entry.ComputedAt).Before(oldestKeptDate)
	})
	if newMergeStats != nil {
		recentMergeStats = append(recentMergeStats, *newMergeStats)
	}
	return recentMergeStats
}

// A date's first entry wins, as a later post that day covers only the hours since the first.
// See state.spec.md § Behaviour
func (r RecentMergeStats) WeekBefore(shown *mergestats.MergeStats) *mergestats.MergeStats {
	if shown == nil {
		return nil
	}
	weekBeforeDate := utcDateAWeekBefore(shown.ComputedAt)
	weekBefore, isFound := utilities.Find(r, isComputedOn(weekBeforeDate))
	if !isFound {
		return nil
	}
	return &weekBefore
}

func (r RecentMergeStats) LastComputedAt() *time.Time {
	if len(r) == 0 {
		return nil
	}
	lastComputedAt := r[len(r)-1].ComputedAt
	return &lastComputedAt
}

func isComputedOn(utcDay time.Time) func(mergestats.MergeStats) bool {
	return func(entry mergestats.MergeStats) bool {
		return utcDate(entry.ComputedAt).Equal(utcDay)
	}
}

func utcDateAWeekBefore(t time.Time) time.Time {
	return utcDate(t).AddDate(0, 0, -daysInAWeek)
}

func utcDate(t time.Time) time.Time {
	year, month, day := t.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func isSamePullRequest(a, b githubclient.PR) bool {
	return a.GetPullRequestRef() == b.GetPullRequestRef()
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
	log.Printf(
		"Saved state to %s with %d PRs and %d cached merged PRs",
		filePath, len(state.TrackedPRs), len(state.MergedPRCache),
	)
	return nil
}
