package messagecontent

import (
	"slices"
	"testing"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/prview"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

var generatedAt = time.Date(2026, 8, 8, 6, 15, 0, 0, time.UTC)

type testPROptions struct {
	number     int
	repository string
	draft      bool
	createdAt  time.Time
	mergedAt   *time.Time
}

func testPR(options testPROptions) prview.PR {
	repository := models.Repository{Owner: "test-org", Name: "test-repo"}
	if options.repository != "" {
		repository = models.Repository{Owner: "test-org", Name: options.repository}
	}
	return prview.PR{
		PR: &githubclient.PR{
			PullRequest: &githubclient.PullRequest{
				Number:    options.number,
				Draft:     options.draft,
				CreatedAt: options.createdAt,
				MergedAt:  options.mergedAt,
			},
			Repository: repository,
		},
	}
}

func assertEqual[T comparable](t *testing.T, what string, got []T, want []T) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("expected %s %v, got %v", what, want, got)
	}
}

func prNumbers(prs []prview.PR) []int {
	return utilities.Map(prs, func(pr prview.PR) int { return pr.GetNumber() })
}

func TestGetContentSortsPRsOldestToNewest(t *testing.T) {
	prs := []prview.PR{
		testPR(testPROptions{number: 7, createdAt: generatedAt.Add(-1 * time.Hour)}),
		testPR(testPROptions{number: 8, createdAt: generatedAt.Add(-10 * time.Hour)}),
	}

	content := GetContent(prs, config.ContentInputs{})

	assertEqual(t, "open PRs", prNumbers(content.PRs), []int{8, 7})
}
