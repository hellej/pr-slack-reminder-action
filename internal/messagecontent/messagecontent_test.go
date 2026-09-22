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
	number      int
	repository  string
	createdAt   time.Time
	mergedAt    *time.Time
	approvers   []string
	commenters  []string
	conflicting bool
	// A reviewer commented or requested changes, which is what puts an unapproved PR on the
	// author.
	hasNonApprovingReview bool
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
				CreatedAt: options.createdAt,
				MergedAt:  options.mergedAt,
				Merged:    options.mergedAt != nil,
			},
			Repository:            repository,
			Conflicting:           options.conflicting,
			HasNonApprovingReview: options.hasNonApprovingReview,
		},
		Approvers:  collaborators(options.approvers),
		Commenters: collaborators(options.commenters),
	}
}

func collaborators(logins []string) []prview.Collaborator {
	return utilities.Map(logins, func(login string) prview.Collaborator {
		return prview.NewCollaborator(githubclient.Collaborator{Login: login}, "")
	})
}

func hoursBefore(hours int) *time.Time {
	at := generatedAt.Add(-time.Duration(hours) * time.Hour)
	return &at
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

func TestGetContentBucketsOpenPRsByNextActionOldestFirst(t *testing.T) {
	openPRs := []prview.PR{
		testPR(testPROptions{number: 1, createdAt: generatedAt.Add(-1 * time.Hour)}),
		testPR(testPROptions{number: 2, createdAt: generatedAt.Add(-9 * time.Hour), approvers: []string{"dana"}}),
		testPR(testPROptions{
			number: 3, createdAt: generatedAt.Add(-5 * time.Hour),
			commenters: []string{"erin"}, hasNonApprovingReview: true,
		}),
		testPR(testPROptions{number: 4, createdAt: generatedAt.Add(-7 * time.Hour)}),
		testPR(testPROptions{
			number: 5, createdAt: generatedAt.Add(-3 * time.Hour), approvers: []string{"dana"}, conflicting: true,
		}),
	}

	content := GetContent(openPRs, nil, nil, time.Time{}, generatedAt, config.ContentInputs{})

	assertEqual(t, "ready to merge", prNumbers(content.ReadyToMerge.PRs), []int{2})
	assertEqual(t, "waiting for author", prNumbers(content.WaitingForAuthor.PRs), []int{3, 5})
	assertEqual(t, "waiting for review", prNumbers(content.WaitingForReview.PRs), []int{4, 1})
	assertEqual(t, "merged", prNumbers(content.Merged.PRs), nil)
}

func TestGetContentGroupsEachSectionByRepositoryInItsOwnOrder(t *testing.T) {
	openPRs := []prview.PR{
		testPR(testPROptions{number: 1, repository: "zebra", createdAt: generatedAt.Add(-9 * time.Hour)}),
		testPR(testPROptions{number: 2, repository: "alpha", createdAt: generatedAt.Add(-4 * time.Hour)}),
		testPR(testPROptions{number: 3, repository: "zebra", createdAt: generatedAt.Add(-2 * time.Hour)}),
	}

	content := GetContent(openPRs, nil, nil, time.Time{}, generatedAt, config.ContentInputs{GroupByRepository: true})

	waitingForReview := content.WaitingForReview
	if len(waitingForReview.PRs) != 0 {
		t.Fatalf("expected no flat PR list when grouping, got %v", prNumbers(waitingForReview.PRs))
	}
	assertEqual(
		t, "grouped repositories",
		utilities.Map(waitingForReview.Groups, func(group PRsOfRepository) string {
			return group.RepositoryName
		}),
		[]string{"zebra", "alpha"},
	)
	assertEqual(t, "zebra PRs", prNumbers(waitingForReview.Groups[0].PRs), []int{1, 3})
}

// The tracked PR merged longest ago is the one the cap would drop if it were counted as
// untracked, and the untracked list arrives oldest first so that keeping it unsorted fails.
func TestGetContentMergesTrackedPRsWithTheNewestUntrackedOnes(t *testing.T) {
	trackedPRs := []prview.PR{
		testPR(testPROptions{number: 1, mergedAt: hoursBefore(200)}),
		testPR(testPROptions{number: 2, createdAt: generatedAt.Add(-3 * time.Hour)}),
		testPR(testPROptions{number: 3, mergedAt: hoursBefore(4)}),
	}
	recentlyMergedPRs := []prview.PR{
		testPR(testPROptions{number: 4, mergedAt: hoursBefore(20)}),
		testPR(testPROptions{number: 5, mergedAt: hoursBefore(11)}),
		testPR(testPROptions{number: 3, mergedAt: hoursBefore(4)}),
		testPR(testPROptions{number: 6, mergedAt: hoursBefore(7)}),
		testPR(testPROptions{number: 7, mergedAt: hoursBefore(2)}),
	}

	content := GetContent(nil, trackedPRs, recentlyMergedPRs, time.Time{}, generatedAt, config.ContentInputs{})

	assertEqual(t, "merged", prNumbers(content.Merged.PRs), []int{7, 3, 6, 5, 1})
}

// Four untracked PRs merged since the post, one more than the cap, and the oldest of them would
// be the one dropped. Before the post, only the newest 3 are kept, and the PR with no merge time
// counts among them, sorted last. The tracked PR merged since the post is in the fetch too, and
// shows once.
func TestGetContentShowsEveryPRMergedSinceThePost(t *testing.T) {
	messagePostedAt := *hoursBefore(10)
	trackedMergedSincePost := testPR(testPROptions{number: 9, mergedAt: hoursBefore(2)})
	recentlyMergedPRs := []prview.PR{
		testPR(testPROptions{number: 1, mergedAt: hoursBefore(9)}),
		testPR(testPROptions{number: 2, mergedAt: hoursBefore(7)}),
		testPR(testPROptions{number: 3, mergedAt: hoursBefore(5)}),
		testPR(testPROptions{number: 4, mergedAt: hoursBefore(3)}),
		testPR(testPROptions{number: 5, mergedAt: hoursBefore(30)}),
		testPR(testPROptions{number: 6, mergedAt: hoursBefore(20)}),
		testPR(testPROptions{number: 7, mergedAt: hoursBefore(40)}),
		testPR(testPROptions{number: 8, mergedAt: hoursBefore(50)}),
		testPR(testPROptions{number: 10}),
		trackedMergedSincePost,
	}

	content := GetContent(
		nil, []prview.PR{trackedMergedSincePost}, recentlyMergedPRs, messagePostedAt, generatedAt,
		config.ContentInputs{},
	)

	assertEqual(t, "merged", prNumbers(content.Merged.PRs), []int{9, 4, 3, 2, 1, 6, 5, 7})
}

func TestGetContentLeavesClosedButNotMergedTrackedPRsOut(t *testing.T) {
	closedPR := testPR(testPROptions{number: 1})
	closedPR.PullRequest.State = "closed"

	content := GetContent(nil, []prview.PR{closedPR}, nil, time.Time{}, generatedAt, config.ContentInputs{})

	if content.HasPRs() {
		t.Errorf("expected no sections, got merged %v", prNumbers(content.Merged.PRs))
	}
}

func TestGetContentSummaryAndNoOpenPRsText(t *testing.T) {
	testCases := []struct {
		name              string
		openPRs           []prview.PR
		trackedPRs        []prview.PR
		expectedSummary   string
		expectedNoOpenPRs string
	}{
		{
			name:            "one open PR",
			openPRs:         []prview.PR{testPR(testPROptions{number: 1})},
			expectedSummary: "1 open PR is waiting for attention 👀",
		},
		{
			name: "two open PRs",
			openPRs: []prview.PR{
				testPR(testPROptions{number: 1}), testPR(testPROptions{number: 2}),
			},
			expectedSummary: "2 open PRs are waiting for attention 👀",
		},
		{
			name:              "only merged PRs",
			trackedPRs:        []prview.PR{testPR(testPROptions{number: 1, mergedAt: hoursBefore(2)})},
			expectedSummary:   "Nothing waiting for review 🎉",
			expectedNoOpenPRs: "All caught up",
		},
		{
			name:              "nothing at all",
			expectedSummary:   "Nothing waiting for review 🎉",
			expectedNoOpenPRs: "All caught up",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			content := GetContent(
				tc.openPRs, tc.trackedPRs, nil, time.Time{}, generatedAt,
				config.ContentInputs{NoPRsMessage: "All caught up"},
			)

			if content.SummaryText != tc.expectedSummary {
				t.Errorf("expected summary %q, got %q", tc.expectedSummary, content.SummaryText)
			}
			if content.NoOpenPRsText != tc.expectedNoOpenPRs {
				t.Errorf("expected no-open-PRs text %q, got %q", tc.expectedNoOpenPRs, content.NoOpenPRsText)
			}
		})
	}
}

func TestGetContentCarriesTheRunTimestamp(t *testing.T) {
	content := GetContent(nil, nil, nil, time.Time{}, generatedAt, config.ContentInputs{})

	if !content.GeneratedAt.Equal(generatedAt) {
		t.Errorf("expected generated at %v, got %v", generatedAt, content.GeneratedAt)
	}
}
