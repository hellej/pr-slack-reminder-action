package main

import (
	"strings"
	"testing"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/canvasbuilder"
	"github.com/hellej/pr-slack-reminder-action/internal/canvascontent"
	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/prparser"
)

func hashTestPR(number int, title string) prparser.PR {
	repository := models.Repository{Owner: "test-org", Name: "test-repo"}
	return prparser.PR{
		PR: &githubclient.PR{
			PullRequest: &githubclient.PullRequest{
				Number:    number,
				Title:     title,
				HTMLURL:   "https://github.com/test-org/test-repo/pull/1",
				CreatedAt: time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC),
			},
			Repository: repository,
		},
		Author: prparser.NewCollaborator(
			githubclient.Collaborator{Login: "alice", Name: "Alice Anderson"}, "",
		),
	}
}

func hashTestContent(openPRs []prparser.PR, generatedAt time.Time) canvascontent.Content {
	return canvascontent.Content{
		OpenPRs:     openPRs,
		GeneratedAt: generatedAt,
	}
}

func TestCanvasContentHashIgnoresTheGeneratedAtTimestamp(t *testing.T) {
	prs := []prparser.PR{hashTestPR(1, "Add pagination to the PR listing")}

	earlier := canvasContentHash(hashTestContent(prs, time.Date(2026, 8, 8, 6, 15, 0, 0, time.UTC)))
	later := canvasContentHash(hashTestContent(prs, time.Date(2026, 8, 9, 11, 45, 0, 0, time.UTC)))

	if earlier != later {
		t.Errorf("Expected the same hash for content differing only in GeneratedAt, got %s and %s", earlier, later)
	}
}

func TestCanvasContentHashChangesWithAPRRow(t *testing.T) {
	generatedAt := time.Date(2026, 8, 8, 6, 15, 0, 0, time.UTC)

	original := canvasContentHash(
		hashTestContent([]prparser.PR{hashTestPR(1, "Add pagination to the PR listing")}, generatedAt),
	)
	retitled := canvasContentHash(
		hashTestContent([]prparser.PR{hashTestPR(1, "Add pagination to the PR list")}, generatedAt),
	)

	if original == retitled {
		t.Errorf("Expected a different hash when a PR row changes, got %s for both", original)
	}
}

// The empty merged section says whether the fetch failed or nothing was merged, so the two
// cases must not be taken for the same canvas.
func TestCanvasContentHashChangesWithUnavailableMergedPRs(t *testing.T) {
	generatedAt := time.Date(2026, 8, 8, 6, 15, 0, 0, time.UTC)
	content := hashTestContent([]prparser.PR{hashTestPR(1, "Add pagination to the PR listing")}, generatedAt)

	withMergedPRs := canvasContentHash(content)
	content.MergedPRsUnavailable = true
	withoutMergedPRs := canvasContentHash(content)

	if withMergedPRs == withoutMergedPRs {
		t.Errorf("Expected a different hash when the merged PRs are unavailable, got %s for both", withMergedPRs)
	}
}

// The hash zeroes GeneratedAt to keep the footer out of it. Zeroing the caller's copy would put
// a year-1 timestamp on the canvas.
func TestCanvasContentHashLeavesTheContentUnmutated(t *testing.T) {
	generatedAt := time.Date(2026, 8, 8, 6, 15, 0, 0, time.UTC)
	content := hashTestContent([]prparser.PR{hashTestPR(1, "Add pagination to the PR listing")}, generatedAt)

	canvasContentHash(content)

	if !content.GeneratedAt.Equal(generatedAt) {
		t.Errorf("Expected GeneratedAt to stay %v, got %v", generatedAt, content.GeneratedAt)
	}
	if !strings.Contains(canvasbuilder.BuildMarkdown(content), "_Updated 2026-08-08 06:15 UTC_") {
		t.Errorf("Expected the real footer to still render, got:\n%s", canvasbuilder.BuildMarkdown(content))
	}
}
