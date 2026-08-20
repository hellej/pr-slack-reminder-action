package canvascontent_test

import (
	"testing"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/canvascontent"
	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/prparser"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

var generatedAt = time.Date(2026, 8, 8, 6, 15, 0, 0, time.UTC)

type testPROptions struct {
	number         int
	repository     string
	draft          bool
	createdAt      time.Time
	lastActivityAt *time.Time
}

func testPR(options testPROptions) prparser.PR {
	repository := models.Repository{Owner: "test-org", Name: "test-repo"}
	if options.repository != "" {
		repository = models.Repository{Owner: "test-org", Name: options.repository}
	}
	return prparser.PR{
		PR: &githubclient.PR{
			PullRequest: &githubclient.PullRequest{
				Number:         options.number,
				Draft:          options.draft,
				CreatedAt:      options.createdAt,
				LastActivityAt: options.lastActivityAt,
			},
			Repository: repository,
		},
	}
}

func activityAgo(age time.Duration) *time.Time {
	timestamp := generatedAt.Add(-age)
	return &timestamp
}

func prNumbers(prs []prparser.PR) []int {
	return utilities.Map(prs, func(pr prparser.PR) int { return pr.GetNumber() })
}

func assertNumbers(t *testing.T, what string, got []int, want []int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected %s %v, got %v", what, want, got)
	}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Fatalf("expected %s %v, got %v", what, want, got)
		}
	}
}

func TestGetContentSplitsDraftsIntoTheWIPSection(t *testing.T) {
	prs := []prparser.PR{
		testPR(testPROptions{number: 1}),
		testPR(testPROptions{number: 2, draft: true, lastActivityAt: activityAgo(time.Hour)}),
		testPR(testPROptions{number: 3}),
	}

	content := canvascontent.GetContent(prs, config.ContentInputs{}, canvascontent.GetContentOptions{
		GeneratedAt: generatedAt,
	})

	assertNumbers(t, "open PRs", prNumbers(content.OpenPRs), []int{1, 3})
	assertNumbers(t, "WIP PRs", prNumbers(content.WIPPRs), []int{2})
}

func TestGetContentKeepsGivenOpenPROrder(t *testing.T) {
	prs := []prparser.PR{
		testPR(testPROptions{number: 7, createdAt: generatedAt.Add(-3 * time.Hour)}),
		testPR(testPROptions{number: 8, createdAt: generatedAt.Add(-30 * time.Hour)}),
	}

	content := canvascontent.GetContent(prs, config.ContentInputs{}, canvascontent.GetContentOptions{
		GeneratedAt: generatedAt,
	})

	assertNumbers(t, "open PRs", prNumbers(content.OpenPRs), []int{7, 8})
}

func TestGetContentGroupsOpenPRsByRepositoryAndKeepsWIPPRsFlat(t *testing.T) {
	prs := []prparser.PR{
		testPR(testPROptions{number: 1, repository: "repo-two"}),
		testPR(testPROptions{number: 2, repository: "repo-one"}),
		testPR(testPROptions{number: 3, repository: "repo-two", draft: true, lastActivityAt: activityAgo(time.Hour)}),
		testPR(testPROptions{number: 4, repository: "repo-one", draft: true, lastActivityAt: activityAgo(2 * time.Hour)}),
	}

	content := canvascontent.GetContent(
		prs,
		config.ContentInputs{GroupByRepository: true},
		canvascontent.GetContentOptions{GeneratedAt: generatedAt},
	)

	if !content.GroupedByRepository {
		t.Error("expected content to be marked as grouped by repository")
	}
	if len(content.OpenPRs) != 0 {
		t.Errorf("expected no flat open PRs when grouping, got %v", prNumbers(content.OpenPRs))
	}
	if len(content.OpenPRsGroupedByRepository) != 2 {
		t.Fatalf("expected 2 repository groups, got %d", len(content.OpenPRsGroupedByRepository))
	}
	firstGroup := content.OpenPRsGroupedByRepository[0]
	if firstGroup.Repository.GetPath() != "test-org/repo-one" {
		t.Errorf("expected first group to be test-org/repo-one, got %s", firstGroup.Repository.GetPath())
	}
	assertNumbers(t, "first group PRs", prNumbers(firstGroup.PRs), []int{2})
	assertNumbers(t, "WIP PRs", prNumbers(content.WIPPRs), []int{3, 4})
}

func TestGetContentSortsWIPPRsByActivityNewestFirst(t *testing.T) {
	// creation order and activity order disagree
	prs := []prparser.PR{
		testPR(testPROptions{
			number: 1, draft: true,
			createdAt: generatedAt.Add(-10 * time.Hour), lastActivityAt: activityAgo(5 * time.Hour),
		}),
		testPR(testPROptions{
			number: 2, draft: true,
			createdAt: generatedAt.Add(-2 * time.Hour), lastActivityAt: activityAgo(9 * time.Hour),
		}),
		testPR(testPROptions{
			number: 3, draft: true,
			createdAt: generatedAt.Add(-6 * time.Hour), lastActivityAt: activityAgo(time.Hour),
		}),
	}

	content := canvascontent.GetContent(prs, config.ContentInputs{}, canvascontent.GetContentOptions{
		GeneratedAt: generatedAt,
	})

	assertNumbers(t, "WIP PRs", prNumbers(content.WIPPRs), []int{3, 1, 2})
}

func TestGetContentExcludesDraftsInactiveForLongerThanTheMaximum(t *testing.T) {
	testCases := []struct {
		name       string
		inactivity time.Duration
		wantKept   bool
	}{
		{name: "just under the maximum", inactivity: canvascontent.MaxDraftPRInactivity - time.Hour, wantKept: true},
		{name: "exactly at the maximum", inactivity: canvascontent.MaxDraftPRInactivity, wantKept: true},
		{name: "just over the maximum", inactivity: canvascontent.MaxDraftPRInactivity + time.Hour, wantKept: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			prs := []prparser.PR{
				testPR(testPROptions{number: 1, draft: true, lastActivityAt: activityAgo(tc.inactivity)}),
			}

			content := canvascontent.GetContent(prs, config.ContentInputs{}, canvascontent.GetContentOptions{
				GeneratedAt: generatedAt,
			})

			if gotKept := len(content.WIPPRs) == 1; gotKept != tc.wantKept {
				t.Errorf("expected draft kept: %v, got kept: %v", tc.wantKept, gotKept)
			}
		})
	}
}

func TestGetContentKeepsDraftWithUnknownActivity(t *testing.T) {
	prs := []prparser.PR{testPR(testPROptions{number: 1, draft: true})}

	content := canvascontent.GetContent(prs, config.ContentInputs{}, canvascontent.GetContentOptions{
		GeneratedAt: generatedAt,
	})

	assertNumbers(t, "WIP PRs", prNumbers(content.WIPPRs), []int{1})
}

func TestGetContentTakesCapFlagsFromOptions(t *testing.T) {
	prs := []prparser.PR{
		testPR(testPROptions{number: 1}),
		testPR(testPROptions{number: 2, draft: true, lastActivityAt: activityAgo(time.Hour)}),
	}

	content := canvascontent.GetContent(prs, config.ContentInputs{}, canvascontent.GetContentOptions{
		GeneratedAt:   generatedAt,
		OpenPRsCapped: true,
		WIPPRsCapped:  true,
	})

	if len(content.OpenPRs) >= githubclient.MaxPRsToFetch || len(content.WIPPRs) >= githubclient.MaxDraftPRsToFetch {
		t.Fatal("expected both sections to hold fewer PRs than their caps")
	}
	if !content.OpenPRsCapped || !content.WIPPRsCapped {
		t.Errorf(
			"expected both cap flags set, got open: %v, WIP: %v", content.OpenPRsCapped, content.WIPPRsCapped,
		)
	}
}

func TestGetContentLeavesCapFlagsUnsetWhenOptionsDont(t *testing.T) {
	prs := []prparser.PR{
		testPR(testPROptions{number: 1}),
		testPR(testPROptions{number: 2, draft: true, lastActivityAt: activityAgo(time.Hour)}),
	}

	content := canvascontent.GetContent(prs, config.ContentInputs{}, canvascontent.GetContentOptions{
		GeneratedAt: generatedAt,
	})

	if content.OpenPRsCapped || content.WIPPRsCapped {
		t.Errorf(
			"expected no cap flags set, got open: %v, WIP: %v", content.OpenPRsCapped, content.WIPPRsCapped,
		)
	}
}

func TestGetContentTakesGeneratedAtFromOptions(t *testing.T) {
	content := canvascontent.GetContent(nil, config.ContentInputs{}, canvascontent.GetContentOptions{
		GeneratedAt: generatedAt,
	})

	if !content.GeneratedAt.Equal(generatedAt) {
		t.Errorf("expected GeneratedAt %v, got %v", generatedAt, content.GeneratedAt)
	}
}
