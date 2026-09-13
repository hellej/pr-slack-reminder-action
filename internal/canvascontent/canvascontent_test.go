package canvascontent_test

import (
	"slices"
	"testing"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/canvascontent"
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
	updatedAt  time.Time
	mergedAt   *time.Time
	// The signals PR.GetNextAction reads.
	approved           bool
	conflicting        bool
	threadWaiting      bool
	nonApprovingReview bool
}

func testPR(options testPROptions) prview.PR {
	repository := models.Repository{Owner: "test-org", Name: "test-repo"}
	if options.repository != "" {
		repository = models.Repository{Owner: "test-org", Name: options.repository}
	}
	var approvers []prview.Collaborator
	if options.approved {
		approvers = []prview.Collaborator{
			prview.NewCollaborator(githubclient.Collaborator{Login: "approver"}, ""),
		}
	}
	return prview.PR{
		PR: &githubclient.PR{
			PullRequest: &githubclient.PullRequest{
				Number:    options.number,
				Draft:     options.draft,
				CreatedAt: options.createdAt,
				UpdatedAt: options.updatedAt,
				MergedAt:  options.mergedAt,
			},
			Repository:                repository,
			Conflicting:               options.conflicting,
			HasThreadWaitingForAuthor: options.threadWaiting,
			HasNonApprovingReview:     options.nonApprovingReview,
		},
		Approvers: approvers,
	}
}

func activityAgo(age time.Duration) time.Time {
	return generatedAt.Add(-age)
}

func mergedAgo(age time.Duration) *time.Time {
	timestamp := generatedAt.Add(-age)
	return &timestamp
}

func prNumbers(prs []prview.PR) []int {
	return utilities.Map(prs, func(pr prview.PR) int { return pr.GetNumber() })
}

func assertEqual[T comparable](t *testing.T, what string, got []T, want []T) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("expected %s %v, got %v", what, want, got)
	}
}

func TestGetContentSplitsDraftsIntoTheWIPSection(t *testing.T) {
	prs := []prview.PR{
		testPR(testPROptions{number: 1}),
		testPR(testPROptions{number: 2, draft: true, updatedAt: activityAgo(time.Hour)}),
		testPR(testPROptions{number: 3}),
	}

	content := canvascontent.GetContent(prs, nil, config.ContentInputs{}, canvascontent.GetContentOptions{
		GeneratedAt: generatedAt,
	})

	assertEqual(t, "open PRs", prNumbers(content.WaitingForReview.PRs), []int{1, 3})
	assertEqual(t, "WIP PRs", prNumbers(content.WIP.PRs), []int{2})
}

func TestGetContentSortsOpenPRsOldestToNewest(t *testing.T) {
	prs := []prview.PR{
		testPR(testPROptions{number: 7, createdAt: generatedAt.Add(-1 * time.Hour)}),
		testPR(testPROptions{number: 8, createdAt: generatedAt.Add(-10 * time.Hour)}),
	}

	content := canvascontent.GetContent(prs, nil, config.ContentInputs{}, canvascontent.GetContentOptions{
		GeneratedAt: generatedAt,
	})

	assertEqual(t, "open PRs", prNumbers(content.WaitingForReview.PRs), []int{8, 7})
}

// The next action decides the section, and the fixtures name the signal rather than the bucket
// so a wrong mapping shows up as a PR in the wrong list.
func TestGetContentBucketsOpenPRsByNextAction(t *testing.T) {
	prs := []prview.PR{
		testPR(testPROptions{number: 1, approved: true}),
		testPR(testPROptions{number: 2, nonApprovingReview: true}),
		testPR(testPROptions{number: 3}),
		testPR(testPROptions{number: 4, approved: true, conflicting: true}),
		testPR(testPROptions{number: 5, threadWaiting: true}),
		testPR(testPROptions{number: 6, conflicting: true}),
		testPR(testPROptions{number: 7, approved: true}),
	}

	content := canvascontent.GetContent(prs, nil, config.ContentInputs{}, canvascontent.GetContentOptions{
		GeneratedAt: generatedAt,
	})

	assertEqual(t, "ready to merge PRs", prNumbers(content.ReadyToMerge.PRs), []int{1, 7})
	assertEqual(t, "PRs waiting for author", prNumbers(content.WaitingForAuthor.PRs), []int{2, 4, 5})
	assertEqual(t, "PRs waiting for review", prNumbers(content.WaitingForReview.PRs), []int{3, 6})
}

// Every bucket is filtered out of one sorted list, so each stays oldest first. The given order
// is newest first in each bucket, so an unsorted implementation fails this.
func TestGetContentKeepsEachOpenBucketOldestFirst(t *testing.T) {
	hoursAgo := func(hours int) time.Time {
		return generatedAt.Add(-time.Duration(hours) * time.Hour)
	}
	prs := []prview.PR{
		testPR(testPROptions{number: 1, approved: true, createdAt: hoursAgo(2)}),
		testPR(testPROptions{number: 2, threadWaiting: true, createdAt: hoursAgo(3)}),
		testPR(testPROptions{number: 3, createdAt: hoursAgo(4)}),
		testPR(testPROptions{number: 4, approved: true, createdAt: hoursAgo(20)}),
		testPR(testPROptions{number: 5, threadWaiting: true, createdAt: hoursAgo(30)}),
		testPR(testPROptions{number: 6, createdAt: hoursAgo(40)}),
	}

	content := canvascontent.GetContent(prs, nil, config.ContentInputs{}, canvascontent.GetContentOptions{
		GeneratedAt: generatedAt,
	})

	assertEqual(t, "ready to merge PRs", prNumbers(content.ReadyToMerge.PRs), []int{4, 1})
	assertEqual(t, "PRs waiting for author", prNumbers(content.WaitingForAuthor.PRs), []int{5, 2})
	assertEqual(t, "PRs waiting for review", prNumbers(content.WaitingForReview.PRs), []int{6, 3})
}

// The repository paths of a grouped section, so its whole group order is one expectation.
func groupPaths(groups []prview.RepositoryPRs) []string {
	return utilities.Map(groups, func(group prview.RepositoryPRs) string {
		return group.Repository.GetPath()
	})
}

// The open PRs come in oldest first, so the oldest PR's repository leads. It is not the
// alphabetically first one here.
func TestGetContentGroupsOpenPRsByRepositoryOldestPRsRepositoryFirst(t *testing.T) {
	prs := []prview.PR{
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
	if len(content.WaitingForReview.PRs) != 0 {
		t.Errorf("expected no flat open PRs when grouping, got %v", prNumbers(content.WaitingForReview.PRs))
	}
	assertEqual(
		t, "open PR group paths", groupPaths(content.WaitingForReview.Groups),
		[]string{"test-org/repo-two", "test-org/repo-one"},
	)
	assertEqual(t, "first group PRs", prNumbers(content.WaitingForReview.Groups[0].PRs), []int{1, 3})
	assertEqual(t, "second group PRs", prNumbers(content.WaitingForReview.Groups[1].PRs), []int{2})
}

// Each bucket is grouped on its own, so one repository can lead two sections and appear under
// both. Nothing dedupes it. The waiting-for-author groups lead with repo-two, which sorts after
// repo-one alphabetically, so alphabetical bucketing fails this too.
func TestGetContentGroupsEachOpenBucketOnItsOwn(t *testing.T) {
	prs := []prview.PR{
		testPR(testPROptions{number: 1, repository: "repo-two", approved: true}),
		testPR(testPROptions{number: 2, repository: "repo-two", threadWaiting: true}),
		testPR(testPROptions{number: 3, repository: "repo-one", threadWaiting: true}),
		testPR(testPROptions{number: 4, repository: "repo-one"}),
	}

	content := canvascontent.GetContent(
		prs,
		nil,
		config.ContentInputs{GroupByRepository: true},
		canvascontent.GetContentOptions{GeneratedAt: generatedAt},
	)

	assertEqual(
		t, "ready to merge group paths", groupPaths(content.ReadyToMerge.Groups),
		[]string{"test-org/repo-two"},
	)
	assertEqual(
		t, "waiting for author group paths", groupPaths(content.WaitingForAuthor.Groups),
		[]string{"test-org/repo-two", "test-org/repo-one"},
	)
	assertEqual(
		t, "waiting for review group paths", groupPaths(content.WaitingForReview.Groups),
		[]string{"test-org/repo-one"},
	)
	if len(content.ReadyToMerge.PRs) != 0 || len(content.WaitingForAuthor.PRs) != 0 {
		t.Error("expected no flat open PRs when grouping")
	}
}

// The WIP PRs are sorted by activity first, so the most recently touched PR's repository leads.
// repo-two sorts after repo-three alphabetically, so alphabetical bucketing fails this.
func TestGetContentGroupsWIPPRsByRepositoryMostRecentActivityRepositoryFirst(t *testing.T) {
	prs := []prview.PR{
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

	if len(content.WIP.PRs) != 0 {
		t.Errorf("expected no flat WIP PRs when grouping, got %v", prNumbers(content.WIP.PRs))
	}
	assertEqual(
		t, "WIP PR group paths", groupPaths(content.WIP.Groups),
		[]string{"test-org/repo-two", "test-org/repo-three"},
	)
	assertEqual(t, "first group PRs", prNumbers(content.WIP.Groups[0].PRs), []int{4})
	assertEqual(t, "second group PRs", prNumbers(content.WIP.Groups[1].PRs), []int{5, 3})
}

func TestGetContentKeepsWIPPRsFlatWithoutGrouping(t *testing.T) {
	prs := []prview.PR{
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

	assertEqual(t, "WIP PRs", prNumbers(content.WIP.PRs), []int{3, 4})
	if len(content.WIP.Groups) != 0 {
		t.Errorf(
			"expected no WIP PR groups without grouping, got %v",
			groupPaths(content.WIP.Groups),
		)
	}
}

func TestGetContentSortsWIPPRsByActivityNewestFirst(t *testing.T) {
	// creation order and activity order disagree
	prs := []prview.PR{
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

	assertEqual(t, "WIP PRs", prNumbers(content.WIP.PRs), []int{3, 1, 2})
}

// Unknown activity is not staleness, so it sorts after every draft with a real update time
// rather than at the old end of them. The unknown draft leads the given order, so an
// unsorted list would leave it first.
func TestGetContentSortsWIPPRsWithUnknownActivityLast(t *testing.T) {
	prs := []prview.PR{
		testPR(testPROptions{number: 1, draft: true}),
		testPR(testPROptions{number: 2, draft: true, updatedAt: activityAgo(9 * time.Hour)}),
		testPR(testPROptions{number: 3, draft: true, updatedAt: activityAgo(time.Hour)}),
	}

	content := canvascontent.GetContent(prs, nil, config.ContentInputs{}, canvascontent.GetContentOptions{
		GeneratedAt: generatedAt,
	})

	assertEqual(t, "WIP PRs", prNumbers(content.WIP.PRs), []int{3, 2, 1})
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
			prs := []prview.PR{
				testPR(testPROptions{number: 1, draft: true, updatedAt: activityAgo(tc.inactivity)}),
			}

			content := canvascontent.GetContent(prs, nil, config.ContentInputs{}, canvascontent.GetContentOptions{
				GeneratedAt: generatedAt,
			})

			if gotKept := len(content.WIP.PRs) == 1; gotKept != tc.wantKept {
				t.Errorf("expected draft kept: %v, got kept: %v", tc.wantKept, gotKept)
			}
		})
	}
}

// Inactive drafts, given oldest activity first, so keeping the first 5 given would keep the
// wrong ones.
func TestGetContentKeepsOnlyTheMostRecentlyActiveInactiveWIPPRs(t *testing.T) {
	prs := []prview.PR{
		testPR(testPROptions{number: 1, draft: true, updatedAt: activityAgo(9 * 24 * time.Hour)}),
		testPR(testPROptions{number: 2, draft: true, updatedAt: activityAgo(8 * 24 * time.Hour)}),
		testPR(testPROptions{number: 3, draft: true, updatedAt: activityAgo(7 * 24 * time.Hour)}),
		testPR(testPROptions{number: 4, draft: true, updatedAt: activityAgo(6 * 24 * time.Hour)}),
		testPR(testPROptions{number: 5, draft: true, updatedAt: activityAgo(5 * 24 * time.Hour)}),
		testPR(testPROptions{number: 6, draft: true, updatedAt: activityAgo(4 * 24 * time.Hour)}),
		testPR(testPROptions{number: 7, draft: true, updatedAt: activityAgo(3 * 24 * time.Hour)}),
	}

	content := canvascontent.GetContent(prs, nil, config.ContentInputs{}, canvascontent.GetContentOptions{
		GeneratedAt: generatedAt,
	})

	assertEqual(t, "WIP PRs", prNumbers(content.WIP.PRs), []int{7, 6, 5, 4, 3})
}

func TestGetContentKeepsFiveInactiveWIPPRs(t *testing.T) {
	prs := []prview.PR{
		testPR(testPROptions{number: 1, draft: true, updatedAt: activityAgo(5 * 24 * time.Hour)}),
		testPR(testPROptions{number: 2, draft: true, updatedAt: activityAgo(4 * 24 * time.Hour)}),
		testPR(testPROptions{number: 3, draft: true, updatedAt: activityAgo(3 * 24 * time.Hour)}),
		testPR(testPROptions{number: 4, draft: true, updatedAt: activityAgo(2 * 24 * time.Hour)}),
		testPR(testPROptions{number: 5, draft: true, updatedAt: activityAgo(25 * time.Hour)}),
	}

	content := canvascontent.GetContent(prs, nil, config.ContentInputs{}, canvascontent.GetContentOptions{
		GeneratedAt: generatedAt,
	})

	assertEqual(t, "WIP PRs", prNumbers(content.WIP.PRs), []int{5, 4, 3, 2, 1})
}

// Recently active drafts are outside the cap, however many there are.
func TestGetContentKeepsEveryRecentlyActiveWIPPR(t *testing.T) {
	var prs []prview.PR
	for i := range 8 {
		prs = append(prs, testPR(testPROptions{
			number: 100 + i, draft: true, updatedAt: activityAgo(time.Duration(i+1) * time.Hour),
		}))
		prs = append(prs, testPR(testPROptions{
			number: 200 + i, draft: true, updatedAt: activityAgo(time.Duration(i+2) * 24 * time.Hour),
		}))
	}

	content := canvascontent.GetContent(prs, nil, config.ContentInputs{}, canvascontent.GetContentOptions{
		GeneratedAt: generatedAt,
	})

	assertEqual(
		t, "WIP PRs", prNumbers(content.WIP.PRs),
		[]int{100, 101, 102, 103, 104, 105, 106, 107, 200, 201, 202, 203, 204},
	)
}

// The cap counts a draft as inactive from 24 hours of silence onwards, measured against
// GeneratedAt. Draft 6 is the candidate, and always the most recently touched one. Counting it
// as inactive fills the cap with drafts 6 to 2 and drops draft 1; counting it as active leaves
// the cap to drafts 5 to 1 and keeps all six.
func TestGetContentCountsWIPPRAsInactiveFromTheActivityThreshold(t *testing.T) {
	testCases := []struct {
		name       string
		inactivity time.Duration
		wantWIPPRs []int
	}{
		{name: "an hour inside the threshold", inactivity: 23 * time.Hour, wantWIPPRs: []int{6, 5, 4, 3, 2, 1}},
		{
			name: "just inside the threshold", inactivity: 24*time.Hour - time.Second,
			wantWIPPRs: []int{6, 5, 4, 3, 2, 1},
		},
		{name: "exactly at the threshold", inactivity: 24 * time.Hour, wantWIPPRs: []int{6, 5, 4, 3, 2, 1}},
		{
			name: "just past the threshold", inactivity: 24*time.Hour + time.Second,
			wantWIPPRs: []int{6, 5, 4, 3, 2},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			prs := []prview.PR{
				testPR(testPROptions{number: 1, draft: true, updatedAt: activityAgo(5 * 24 * time.Hour)}),
				testPR(testPROptions{number: 2, draft: true, updatedAt: activityAgo(4 * 24 * time.Hour)}),
				testPR(testPROptions{number: 3, draft: true, updatedAt: activityAgo(3 * 24 * time.Hour)}),
				testPR(testPROptions{number: 4, draft: true, updatedAt: activityAgo(2 * 24 * time.Hour)}),
				testPR(testPROptions{number: 5, draft: true, updatedAt: activityAgo(36 * time.Hour)}),
				testPR(testPROptions{number: 6, draft: true, updatedAt: activityAgo(tc.inactivity)}),
			}

			content := canvascontent.GetContent(prs, nil, config.ContentInputs{}, canvascontent.GetContentOptions{
				GeneratedAt: generatedAt,
			})

			assertEqual(t, "WIP PRs", prNumbers(content.WIP.PRs), tc.wantWIPPRs)
		})
	}
}

// Unknown activity is not inactivity, so the cap never drops such a draft.
func TestGetContentKeepsDraftWithUnknownActivityBesidesTheCappedInactiveOnes(t *testing.T) {
	prs := []prview.PR{
		testPR(testPROptions{number: 1, draft: true, updatedAt: activityAgo(6 * 24 * time.Hour)}),
		testPR(testPROptions{number: 2, draft: true, updatedAt: activityAgo(5 * 24 * time.Hour)}),
		testPR(testPROptions{number: 3, draft: true, updatedAt: activityAgo(4 * 24 * time.Hour)}),
		testPR(testPROptions{number: 4, draft: true, updatedAt: activityAgo(3 * 24 * time.Hour)}),
		testPR(testPROptions{number: 5, draft: true, updatedAt: activityAgo(2 * 24 * time.Hour)}),
		testPR(testPROptions{number: 6, draft: true}),
	}

	content := canvascontent.GetContent(prs, nil, config.ContentInputs{}, canvascontent.GetContentOptions{
		GeneratedAt: generatedAt,
	})

	assertEqual(t, "WIP PRs", prNumbers(content.WIP.PRs), []int{5, 4, 3, 2, 1, 6})
}

// Drafts past MaxDraftPRInactivity never take one of the kept slots.
func TestGetContentKeepsNoStaleDraftAmongTheCappedInactiveOnes(t *testing.T) {
	prs := []prview.PR{
		testPR(testPROptions{number: 1, draft: true, updatedAt: activityAgo(70 * 24 * time.Hour)}),
		testPR(testPROptions{number: 2, draft: true, updatedAt: activityAgo(65 * 24 * time.Hour)}),
		testPR(testPROptions{number: 3, draft: true, updatedAt: activityAgo(61 * 24 * time.Hour)}),
		testPR(testPROptions{number: 4, draft: true, updatedAt: activityAgo(10 * 24 * time.Hour)}),
		testPR(testPROptions{number: 5, draft: true, updatedAt: activityAgo(9 * 24 * time.Hour)}),
		testPR(testPROptions{number: 6, draft: true, updatedAt: activityAgo(8 * 24 * time.Hour)}),
		testPR(testPROptions{number: 7, draft: true, updatedAt: activityAgo(7 * 24 * time.Hour)}),
		testPR(testPROptions{number: 8, draft: true, updatedAt: activityAgo(6 * 24 * time.Hour)}),
	}

	content := canvascontent.GetContent(prs, nil, config.ContentInputs{}, canvascontent.GetContentOptions{
		GeneratedAt: generatedAt,
	})

	assertEqual(t, "WIP PRs", prNumbers(content.WIP.PRs), []int{8, 7, 6, 5, 4})
}

// The cap is global, so three repositories with three inactive drafts each yield five rows
// in total rather than five per repository.
func TestGetContentCapsInactiveWIPPRsAcrossRepositories(t *testing.T) {
	prs := []prview.PR{
		testPR(testPROptions{
			number: 1, repository: "repo-one", draft: true, updatedAt: activityAgo(9 * 24 * time.Hour),
		}),
		testPR(testPROptions{
			number: 2, repository: "repo-one", draft: true, updatedAt: activityAgo(6 * 24 * time.Hour),
		}),
		testPR(testPROptions{
			number: 3, repository: "repo-one", draft: true, updatedAt: activityAgo(3 * 24 * time.Hour),
		}),
		testPR(testPROptions{
			number: 4, repository: "repo-two", draft: true, updatedAt: activityAgo(8 * 24 * time.Hour),
		}),
		testPR(testPROptions{
			number: 5, repository: "repo-two", draft: true, updatedAt: activityAgo(5 * 24 * time.Hour),
		}),
		testPR(testPROptions{
			number: 6, repository: "repo-two", draft: true, updatedAt: activityAgo(2 * 24 * time.Hour),
		}),
		testPR(testPROptions{
			number: 7, repository: "repo-three", draft: true, updatedAt: activityAgo(7 * 24 * time.Hour),
		}),
		testPR(testPROptions{
			number: 8, repository: "repo-three", draft: true, updatedAt: activityAgo(4 * 24 * time.Hour),
		}),
		testPR(testPROptions{
			number: 9, repository: "repo-three", draft: true, updatedAt: activityAgo(25 * time.Hour),
		}),
	}

	content := canvascontent.GetContent(
		prs,
		nil,
		config.ContentInputs{GroupByRepository: true},
		canvascontent.GetContentOptions{GeneratedAt: generatedAt},
	)

	assertEqual(
		t, "WIP PR group paths", groupPaths(content.WIP.Groups),
		[]string{"test-org/repo-three", "test-org/repo-two", "test-org/repo-one"},
	)
	assertEqual(t, "first group PRs", prNumbers(content.WIP.Groups[0].PRs), []int{9, 8})
	assertEqual(t, "second group PRs", prNumbers(content.WIP.Groups[1].PRs), []int{6, 5})
	assertEqual(t, "third group PRs", prNumbers(content.WIP.Groups[2].PRs), []int{3})
}

func TestGetContentKeepsDraftWithUnknownActivity(t *testing.T) {
	prs := []prview.PR{testPR(testPROptions{number: 1, draft: true})}

	content := canvascontent.GetContent(prs, nil, config.ContentInputs{}, canvascontent.GetContentOptions{
		GeneratedAt: generatedAt,
	})

	assertEqual(t, "WIP PRs", prNumbers(content.WIP.PRs), []int{1})
}

func TestGetContentTakesCapFlagsFromOptions(t *testing.T) {
	prs := []prview.PR{
		testPR(testPROptions{number: 1}),
		testPR(testPROptions{number: 2, draft: true, updatedAt: activityAgo(time.Hour)}),
	}

	content := canvascontent.GetContent(prs, nil, config.ContentInputs{}, canvascontent.GetContentOptions{
		GeneratedAt:   generatedAt,
		OpenPRsCapped: true,
		WIPPRsCapped:  true,
	})

	if len(content.WaitingForReview.PRs) >= githubclient.MaxPRsToFetch || len(content.WIP.PRs) >= githubclient.MaxDraftPRsToFetch {
		t.Fatal("expected both sections to hold fewer PRs than their caps")
	}
	if !content.OpenPRsCapped || !content.WIPPRsCapped {
		t.Errorf(
			"expected both cap flags set, got open: %v, WIP: %v", content.OpenPRsCapped, content.WIPPRsCapped,
		)
	}
}

func TestGetContentLeavesCapFlagsUnsetWhenOptionsDont(t *testing.T) {
	prs := []prview.PR{
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
	mergedPRs := []prview.PR{
		testPR(testPROptions{number: 1, mergedAt: mergedAgo(3 * 24 * time.Hour)}),
		testPR(testPROptions{number: 2, mergedAt: mergedAgo(2 * time.Hour)}),
		testPR(testPROptions{number: 3, mergedAt: mergedAgo(24 * time.Hour)}),
	}

	content := canvascontent.GetContent(
		nil, mergedPRs, config.ContentInputs{}, canvascontent.GetContentOptions{
			GeneratedAt: generatedAt,
		},
	)

	assertEqual(t, "merged PRs", prNumbers(content.Merged.PRs), []int{2, 3, 1})
}

// The merged PRs are sorted by merge time first, so the most recently merged PR's repository
// leads. It is not the alphabetically first one here.
func TestGetContentGroupsMergedPRsByRepositoryNewestMergesRepositoryFirst(t *testing.T) {
	mergedPRs := []prview.PR{
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

	if len(content.Merged.PRs) != 0 {
		t.Errorf("expected no flat merged PRs when grouping, got %v", prNumbers(content.Merged.PRs))
	}
	assertEqual(
		t, "merged PR group paths", groupPaths(content.Merged.Groups),
		[]string{"test-org/repo-two", "test-org/repo-one"},
	)
	assertEqual(t, "first group PRs", prNumbers(content.Merged.Groups[0].PRs), []int{2})
	assertEqual(t, "second group PRs", prNumbers(content.Merged.Groups[1].PRs), []int{1, 3})
}

func TestGetContentKeepsMergedPRsFlatWithoutGrouping(t *testing.T) {
	mergedPRs := []prview.PR{
		testPR(testPROptions{number: 1, repository: "repo-one", mergedAt: mergedAgo(time.Hour)}),
		testPR(testPROptions{number: 2, repository: "repo-two", mergedAt: mergedAgo(2 * time.Hour)}),
	}

	content := canvascontent.GetContent(
		nil, mergedPRs, config.ContentInputs{}, canvascontent.GetContentOptions{
			GeneratedAt: generatedAt,
		},
	)

	assertEqual(t, "merged PRs", prNumbers(content.Merged.PRs), []int{1, 2})
	if len(content.Merged.Groups) != 0 {
		t.Errorf(
			"expected no merged PR groups without grouping, got %v",
			groupPaths(content.Merged.Groups),
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
