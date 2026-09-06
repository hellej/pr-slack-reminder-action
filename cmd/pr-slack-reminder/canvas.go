package main

import (
	"context"
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
	"github.com/hellej/pr-slack-reminder-action/internal/prparser"
)

// Refreshes the canvas, and fetches the merged PRs itself, so that both run modes share one code
// path and one "now". A failed merged fetch costs that section only: the canvas is written without
// it and the error is carried out to the run's exit code.
//
// openPRs carries post mode's fetch, which the message path already made. Nil asks for a fetch
// here, which is what update mode always needs: its message is state-tracked, so it never lists
// what is open right now.
//
// previousHash is the hash of what the last run put on the canvas. Matching it skips the write:
// Slack's canvas client mis-merges a replace that lands while somebody has the canvas open.
// Returns the hash that is on the canvas afterwards, which is the previous one on every path
// that wrote nothing, a failed write included.
func refreshPRTrackerCanvas(
	githubClient githubclient.Client,
	slackClient slackclient.Client,
	openPRs *githubclient.OpenPRsResult,
	cfg config.Config,
	previousHash string,
) (string, error) {
	generatedAt := time.Now().UTC()

	if openPRs == nil {
		fetched, err := findOpenPRs(githubClient, cfg, githubclient.PRFetchOptions{IncludeDrafts: true})
		if err != nil {
			return previousHash, err
		}
		openPRs = &fetched
	}

	mergedPRs, mergedPRsErr := findRecentlyMergedPRs(githubClient, cfg, generatedAt)
	if mergedPRsErr != nil {
		log.Printf("Failed to fetch recently merged PRs: %v", mergedPRsErr)
	}

	parsedPRs := prparser.ParsePRs(openPRs.PRs, cfg.ContentInputs)
	content := canvascontent.GetContent(
		parsedPRs,
		prparser.ParsePRs(mergedPRs, cfg.ContentInputs),
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

func findRecentlyMergedPRs(
	githubClient githubclient.Client, cfg config.Config, generatedAt time.Time,
) ([]githubclient.PR, error) {
	ctx, cancel := context.WithTimeout(context.Background(), prFetchTimeout)
	defer cancel()
	return githubClient.FindRecentlyMergedPRs(
		ctx,
		cfg.Repositories,
		cfg.GetFiltersForRepository,
		generatedAt.Add(-githubclient.RecentlyMergedWindow),
	)
}
