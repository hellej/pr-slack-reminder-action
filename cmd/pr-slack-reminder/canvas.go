package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/slackclient"
	"github.com/hellej/pr-slack-reminder-action/internal/canvasbuilder"
	"github.com/hellej/pr-slack-reminder-action/internal/canvascontent"
	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/internal/prview"
)

// A failed merged fetch costs the merged section only: the canvas is written without it, and
// mergedPRsErr is returned so that it still reaches the run's exit code.
//
// previousHash is the hash of what the last run put on the canvas. Matching it skips the write:
// Slack's canvas client mis-merges a replace that lands while somebody has the canvas open.
// Returns the hash that is on the canvas afterwards, which is the previous one on every path
// that wrote nothing, a failed write included.
func refreshPRTrackerCanvas(
	slackClient slackclient.Client,
	cfg config.Config,
	openPRs githubclient.OpenPRsResult,
	mergedPRs []githubclient.PR,
	mergedPRsErr error,
	generatedAt time.Time,
	previousHash string,
) (string, error) {
	prViews := prview.BuildPRViews(openPRs.PRs, cfg.ContentInputs)
	content := canvascontent.GetContent(
		prViews,
		prview.BuildPRViews(mergedPRs, cfg.ContentInputs),
		cfg.ContentInputs,
		canvascontent.GetContentOptions{
			OpenPRsCapped:        openPRs.OpenPRsCapped,
			WIPPRsCapped:         openPRs.DraftPRsCapped,
			MergedPRsUnavailable: mergedPRsErr != nil,
			GeneratedAt:          generatedAt,
		},
	)

	contentHash := canvasContentHash(content)
	if contentHash == previousHash {
		log.Println("Canvas content is unchanged since the last run, leaving the canvas as it is")
		return previousHash, mergedPRsErr
	}

	writeErr := slackClient.ReplaceCanvasContent(
		cfg.PRTrackerCanvasID, canvasbuilder.BuildMarkdown(content),
	)
	if writeErr != nil {
		return previousHash, errors.Join(writeErr, mergedPRsErr)
	}
	return contentHash, mergedPRsErr
}

// Hashes the markdown the canvas would get, with the footer timestamp left out so that a run
// rendering the same rows recognizes its own canvas.
func canvasContentHash(content canvascontent.Content) string {
	contentWithoutTimestamp := content
	contentWithoutTimestamp.GeneratedAt = time.Time{}
	digest := sha256.Sum256([]byte(canvasbuilder.BuildMarkdown(contentWithoutTimestamp)))
	return hex.EncodeToString(digest[:])
}
