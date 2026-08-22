package main

import (
	"context"
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
func refreshPRTrackerCanvas(
	githubClient githubclient.Client,
	slackClient slackclient.Client,
	openPRs *githubclient.OpenPRsResult,
	cfg config.Config,
) error {
	generatedAt := time.Now().UTC()

	if openPRs == nil {
		fetched, err := findOpenPRs(githubClient, cfg, githubclient.PRFetchOptions{IncludeDrafts: true})
		if err != nil {
			return err
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

	writeErr := slackClient.ReplaceCanvasContent(
		cfg.PRTrackerCanvasID, canvasbuilder.BuildMarkdown(content),
	)
	return errors.Join(writeErr, mergedPRsErr)
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
