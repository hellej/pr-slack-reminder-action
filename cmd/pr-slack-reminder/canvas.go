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

// See run.spec.md for this function's full behaviour.
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

func canvasContentHash(content canvascontent.Content) string {
	contentWithoutTimestamp := content
	contentWithoutTimestamp.GeneratedAt = time.Time{}
	digest := sha256.Sum256([]byte(canvasbuilder.BuildMarkdown(contentWithoutTimestamp)))
	return hex.EncodeToString(digest[:])
}
