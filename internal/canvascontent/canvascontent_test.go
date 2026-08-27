package canvascontent_test

import (
	"slices"
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
	number     int
	repository string
	draft      bool
	createdAt  time.Time
	updatedAt  time.Time
	mergedAt   *time.Time
}

func testPR(options testPROptions) prparser.PR {
	repository := models.Repository{Owner: "test-org", Name: "test-repo"}
	if options.repository != "" {
		repository = models.Repository{Owner: "test-org", Name: options.repository}
	}
	return prparser.PR{
		PR: &githubclient.PR{
			PullRequest: &githubclient.PullRequest{
				Number:    options.number,
				Draft:     options.draft,
				CreatedAt: options.createdAt,
				UpdatedAt: options.updatedAt,
				MergedAt:  options.mergedAt,
			},
			Repository: repository,
		},
	}
}

func activityAgo(age time.Duration) time.Time {
	return generatedAt.Add(-age)
}

func mergedAgo(age time.Duration) *time.Time {
	timestamp := generatedAt.Add(-age)
	return &timestamp
}

func prNumbers(prs []prparser.PR) []int {
	return utilities.Map(prs, func(pr prparser.PR) int { return pr.GetNumber() })
}

func assertEqual[T comparable](t *testing.T, what string, got []T, want []T) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("expected %s %v, got %v", what, want, got)
	}
}

func TestGetContentSplitsDraftsIntoTheWIPSection(t *testing.T) {
	prs := []prparser.PR{
		testPR(testPROptions{number: 1}),
		testPR(testPROptions{number: 2, draft: true, updatedAt: activityAgo(time.Hour)}),
		testPR(testPROptions{number: 3}),
	}

	content := canvascontent.GetContent(prs, nil, config.ContentInputs{}, canvascontent.GetContentOptions{
		GeneratedAt: generatedAt,
	})

	assertEqual(t, "open PRs", prNumbers(content.OpenPRs), []int{1, 3})
	assertEqual(t, "WIP PRs", prNumbers(content.WIPPRs), []int{2})
}

func TestGetContentSortsOpenPRsOldestToNewest(t *testing.T) {
	prs := []prparser.PR{
		testPR(testPROptions{number: 7, createdAt: generatedAt.Add(-1 * time.Hour)}),
		testPR(testPROptions{number: 8, createdAt: generatedAt.Add(-10 * time.Hour)}),
	}

	content := canvascontent.GetContent(prs, nil, config.ContentInputs{}, canvascontent.GetContentOptions{
		GeneratedAt: generatedAt,
	})

	assertEqual(t, "open PRs", prNumbers(content.OpenPRs), []int{8, 7})
}

// The repository paths of a grouped section, so its whole group order is one expectation.
func groupPaths(groups []prparser.RepositoryPRs) []string {
	return utilities.Map(groups, func(group prparser.RepositoryPRs) string {
		return group.Repository.GetPath()
	})
}

// The open PRs come in oldest first, so the oldest PR's repository leads. It is not the
// alphabetically first one here.
func TestGetContentGroupsOpenPRsByRepositoryOldestPRsRepositoryFirst(t *testing.T) {
	prs := []prparser.PR{
		testPR(testPROptions{number: 1, repository: "repo-two"}),
		testPR(testPROptions{number: 2, repository: "repo-one"}),
		testPR(testPROptions{number: 3, repository: "repo-two"}),
	}

	content := canvascontent.GetContent(
		prs,
		nil,
		config.ContentInputs{GroupByRepository: true},
		canvascontent.GetContentOptions{GeneratedAt: generatedAt},
	)

	if !content.GroupedByRepository {
		t.Error("expected content to be marked as grouped by repository")
	}
	if len(content.OpenPRs) != 0 {
		t.Errorf("expected no flat open PRs when grouping, got %v", prNumbers(content.OpenPRs))
	}
	assertEqual(
		t, "open PR group paths", groupPaths(content.OpenPRsGroupedByRepository),
		[]string{"test-org/repo-two", "test-org/repo-one"},
	)
	assertEqual(t, "first group PRs", prNumbers(content.OpenPRsGroupedByRepository[0].PRs), []int{1, 3})
	assertEqual(t, "second group PRs", prNumbers(content.OpenPRsGroupedByRepository[1].PRs), []int{2})
}

// The WIP PRs are sorted by activity first, so the most recently touched PR's repository leads.
// repo-two sorts after repo-three alphabetically, so alphabetical bucketing fails this.
func TestGetContentGroupsWIPPRsByRepositoryMostRecentActivityRepositoryFirst(t *testing.T) {
	prs := []prparser.PR{
		testPR(testPROptions{number: 1, repository: "repo-two"}),
		testPR(testPROptions{number: 2, repository: "repo-one"}),
		testPR(testPROptions{
			number: 3, repository: "repo-three", draft: true, updatedAt: activityAgo(3 * time.Hour),
		}),
		testPR(testPROptions{
			number: 4, repository: "repo-two", draft: true, updatedAt: activityAgo(time.Hour),
		}),
		testPR(testPROptions{
			number: 5, repository: "repo-three", draft: true, updatedAt: activityAgo(2 * time.Hour),
		}),
	}

	content := canvascontent.GetContent(
		prs,
		nil,
		config.ContentInputs{GroupByRepository: true},
		canvascontent.GetContentOptions{GeneratedAt: generatedAt},
	)

	if len(content.WIPPRs) != 0 {
		t.Errorf("expected no flat WIP PRs when grouping, got %v", prNumbers(content.WIPPRs))
	}
	assertEqual(
		t, "WIP PR group paths", groupPaths(content.WIPPRsGroupedByRepository),
		[]string{"test-org/repo-two", "test-org/repo-three"},
	)
	assertEqual(t, "first group PRs", prNumbers(content.WIPPRsGroupedByRepository[0].PRs), []int{4})
	assertEqual(t, "second group PRs", prNumbers(content.WIPPRsGroupedByRepository[1].PRs), []int{5, 3})
}

func TestGetContentKeepsWIPPRsFlatWithoutGrouping(t *testing.T) {
	prs := []prparser.PR{
		testPR(testPROptions{
			number: 3, repository: "repo-two", draft: true, updatedAt: activityAgo(time.Hour),
		}),
		testPR(testPROptions{
			number: 4, repository: "repo-one", draft: true, updatedAt: activityAgo(2 * time.Hour),
		}),
	}

	content := canvascontent.GetContent(prs, nil, config.ContentInputs{}, canvascontent.GetContentOptions{
		GeneratedAt: generatedAt,
	})

	assertEqual(t, "WIP PRs", prNumbers(content.WIPPRs), []int{3, 4})
	if len(content.WIPPRsGroupedByRepository) != 0 {
		t.Errorf(
			"expected no WIP PR groups without grouping, got %v",
			groupPaths(content.WIPPRsGroupedByRepository),
		)
	}
}

func TestGetContentSortsWIPPRsByActivityNewestFirst(t *testing.T) {
	// creation order and activity order disagree
	prs := []prparser.PR{
		testPR(testPROptions{
			number: 1, draft: true,
			createdAt: generatedAt.Add(-10 * time.Hour), updatedAt: activityAgo(5 * time.Hour),
		}),
		testPR(testPROptions{
			number: 2, draft: true,
			createdAt: generatedAt.Add(-2 * time.Hour), updatedAt: activityAgo(9 * time.Hour),
		}),
		testPR(testPROptions{
			number: 3, draft: true,
			createdAt: generatedAt.Add(-6 * time.Hour), updatedAt: activityAgo(time.Hour),
		}),
	}

	content := canvascontent.GetContent(prs, nil, config.ContentInputs{}, canvascontent.GetContentOptions{
		GeneratedAt: generatedAt,
	})

	assertEqual(t, "WIP PRs", prNumbers(content.WIPPRs), []int{3, 1, 2})
}

// Unknown activity is not staleness, so it sorts after every draft with a real update time
// rather than at the old end of them. The unknown draft leads the given order, so an
// unsorted list would leave it first.
func TestGetContentSortsWIPPRsWithUnknownActivityLast(t *testing.T) {
	prs := []prparser.PR{
		testPR(testPROptions{number: 1, draft: true}),
		testPR(testPROptions{number: 2, draft: true, updatedAt: activityAgo(9 * time.Hour)}),
		testPR(testPROptions{number: 3, draft: true, updatedAt: activityAgo(time.Hour)}),
	}

	content := canvascontent.GetContent(prs, nil, config.ContentInputs{}, canvascontent.GetContentOptions{
		GeneratedAt: generatedAt,
	})

	assertEqual(t, "WIP PRs", prNumbers(content.WIPPRs), []int{3, 2, 1})
}

func TestGetContentExcludesDraftsInactiveForLongerThanTheMaximum(t *testing.T) {
	testCases := []struct {
		name       string
		inactivity time.Duration
		wantKept   bool
	}{
		{name: "a day inside the maximum", inactivity: 59 * 24 * time.Hour, wantKept: true},
		{name: "just under the maximum", inactivity: canvascontent.MaxDraftPRInactivity - time.Hour, wantKept: true},
		{name: "exactly at the maximum", inactivity: canvascontent.MaxDraftPRInactivity, wantKept: true},
		{name: "just over the maximum", inactivity: canvascontent.MaxDraftPRInactivity + time.Hour, wantKept: false},
		{name: "a day past the maximum", inactivity: 61 * 24 * time.Hour, wantKept: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			prs := []prparser.PR{
				testPR(testPROptions{number: 1, draft: true, updatedAt: activityAgo(tc.inactivity)}),
			}

			content := canvascontent.GetContent(prs, nil, config.ContentInputs{}, canvascontent.GetContentOptions{
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

	content := canvascontent.GetContent(prs, nil, config.ContentInputs{}, canvascontent.GetContentOptions{
		GeneratedAt: generatedAt,
	})

	assertEqual(t, "WIP PRs", prNumbers(content.WIPPRs), []int{1})
}

func TestGetContentTakesCapFlagsFromOptions(t *testing.T) {
	prs := []prparser.PR{
		testPR(testPROptions{number: 1}),
		testPR(testPROptions{number: 2, draft: true, updatedAt: activityAgo(time.Hour)}),
	}

	content := canvascontent.GetContent(prs, nil, config.ContentInputs{}, canvascontent.GetContentOptions{
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
		testPR(testPROptions{number: 2, draft: true, updatedAt: activityAgo(time.Hour)}),
	}

	content := canvascontent.GetContent(prs, nil, config.ContentInputs{}, canvascontent.GetContentOptions{
		GeneratedAt: generatedAt,
	})

	if content.OpenPRsCapped || content.WIPPRsCapped {
		t.Errorf(
			"expected no cap flags set, got open: %v, WIP: %v", content.OpenPRsCapped, content.WIPPRsCapped,
		)
	}
}

// The merged PRs come from their own fetch, so they are never derived from the open PR list.
func TestGetContentSortsMergedPRsNewestMergeFirst(t *testing.T) {
	mergedPRs := []prparser.PR{
		testPR(testPROptions{number: 1, mergedAt: mergedAgo(3 * 24 * time.Hour)}),
		testPR(testPROptions{number: 2, mergedAt: mergedAgo(2 * time.Hour)}),
		testPR(testPROptions{number: 3, mergedAt: mergedAgo(24 * time.Hour)}),
	}

	content := canvascontent.GetContent(
		nil, mergedPRs, config.ContentInputs{}, canvascontent.GetContentOptions{
			GeneratedAt: generatedAt,
		},
	)

	assertEqual(t, "merged PRs", prNumbers(content.MergedPRs), []int{2, 3, 1})
}

// The merged PRs are sorted by merge time first, so the most recently merged PR's repository
// leads. It is not the alphabetically first one here.
func TestGetContentGroupsMergedPRsByRepositoryNewestMergesRepositoryFirst(t *testing.T) {
	mergedPRs := []prparser.PR{
		testPR(testPROptions{number: 1, repository: "repo-one", mergedAt: mergedAgo(3 * time.Hour)}),
		testPR(testPROptions{number: 2, repository: "repo-two", mergedAt: mergedAgo(time.Hour)}),
		testPR(testPROptions{number: 3, repository: "repo-one", mergedAt: mergedAgo(4 * time.Hour)}),
	}

	content := canvascontent.GetContent(
		nil,
		mergedPRs,
		config.ContentInputs{GroupByRepository: true},
		canvascontent.GetContentOptions{GeneratedAt: generatedAt},
	)

	if len(content.MergedPRs) != 0 {
		t.Errorf("expected no flat merged PRs when grouping, got %v", prNumbers(content.MergedPRs))
	}
	assertEqual(
		t, "merged PR group paths", groupPaths(content.MergedPRsGroupedByRepository),
		[]string{"test-org/repo-two", "test-org/repo-one"},
	)
	assertEqual(t, "first group PRs", prNumbers(content.MergedPRsGroupedByRepository[0].PRs), []int{2})
	assertEqual(t, "second group PRs", prNumbers(content.MergedPRsGroupedByRepository[1].PRs), []int{1, 3})
}

func TestGetContentKeepsMergedPRsFlatWithoutGrouping(t *testing.T) {
	mergedPRs := []prparser.PR{
		testPR(testPROptions{number: 1, repository: "repo-one", mergedAt: mergedAgo(time.Hour)}),
		testPR(testPROptions{number: 2, repository: "repo-two", mergedAt: mergedAgo(2 * time.Hour)}),
	}

	content := canvascontent.GetContent(
		nil, mergedPRs, config.ContentInputs{}, canvascontent.GetContentOptions{
			GeneratedAt: generatedAt,
		},
	)

	assertEqual(t, "merged PRs", prNumbers(content.MergedPRs), []int{1, 2})
	if len(content.MergedPRsGroupedByRepository) != 0 {
		t.Errorf(
			"expected no merged PR groups without grouping, got %v",
			groupPaths(content.MergedPRsGroupedByRepository),
		)
	}
}

func TestGetContentTakesMergedPRsUnavailableFromOptions(t *testing.T) {
	content := canvascontent.GetContent(
		nil, nil, config.ContentInputs{}, canvascontent.GetContentOptions{
			GeneratedAt:          generatedAt,
			MergedPRsUnavailable: true,
		},
	)

	if !content.MergedPRsUnavailable {
		t.Error("expected the merged PRs to be reported as unavailable")
	}
}

func TestGetContentTakesGeneratedAtFromOptions(t *testing.T) {
	content := canvascontent.GetContent(nil, nil, config.ContentInputs{}, canvascontent.GetContentOptions{
		GeneratedAt: generatedAt,
	})

	if !content.GeneratedAt.Equal(generatedAt) {
		t.Errorf("expected GeneratedAt %v, got %v", generatedAt, content.GeneratedAt)
	}
}
