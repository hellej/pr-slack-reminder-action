package prview_test

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

func testPR(number int, createdAt, updatedAt time.Time) githubclient.PR {
	return githubclient.PR{
		PullRequest: &githubclient.PullRequest{
			Number:    number,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
			Author:    githubclient.Collaborator{Login: "author"},
		},
	}
}

func testPRView(number int, createdAt, updatedAt time.Time) prview.PR {
	return prview.PR{
		PR: &githubclient.PR{
			PullRequest: &githubclient.PullRequest{
				Number:    number,
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
				Author:    githubclient.Collaborator{Login: "author"},
			},
		},
	}
}

func TestBuildPRViewsReturnsGivenOrder(t *testing.T) {
	now := time.Now()
	pr1 := testPR(1, now.Add(-30*time.Hour), now.Add(-30*time.Hour))
	pr3 := testPR(2, now.Add(-1*time.Hour), now.Add(-1*time.Hour))
	pr2 := testPR(3, now.Add(-40*time.Hour), now.Add(-40*time.Hour))

	prViews := prview.BuildPRViews([]githubclient.PR{pr1, pr3, pr2}, config.ContentInputs{})

	want := []int{1, 2, 3}
	got := utilities.Map(prViews, func(pr prview.PR) int { return pr.GetNumber() })

	if !slices.Equal(got, want) {
		t.Errorf("expected PR views to be in order %v, got %v", want, got)
	}
}

func TestBuildPRViewsIsOldPRFlagSetCorrectly(t *testing.T) {
	now := time.Now()
	pr1 := testPR(1, now.Add(-1*time.Hour), now.Add(-1*time.Hour))
	pr3 := testPR(2, now.Add(-30*time.Hour), now.Add(-30*time.Hour))
	pr2 := testPR(3, now.Add(-40*time.Hour), now.Add(-40*time.Hour))

	prViews := prview.BuildPRViews(
		[]githubclient.PR{pr1, pr3, pr2},
		config.ContentInputs{OldPRThresholdHours: 35},
	)

	isOld := func(pr prview.PR) bool {
		return pr.IsOldPR
	}

	want := []bool{false, false, true}
	got := utilities.Map(prViews, isOld)

	if !slices.Equal(got, want) {
		t.Errorf("expected PR views to have IsOldPR flags %v, got %v", want, got)
	}

}

func TestSortPRsOldestToNewest(t *testing.T) {
	now := time.Now()
	oldest := testPRView(1, now.Add(-48*time.Hour), now.Add(-48*time.Hour))
	middle := testPRView(2, now.Add(-24*time.Hour), now.Add(-24*time.Hour))
	newest := testPRView(3, now.Add(-1*time.Hour), now.Add(-1*time.Hour))

	result := prview.SortPRsOldestToNewest([]prview.PR{newest, oldest, middle})

	want := []int{1, 2, 3}
	for i, number := range want {
		if result[i].GetNumber() != number {
			t.Errorf("expected PR at position %d to be #%d, got #%d", i, number, result[i].GetNumber())
		}
	}
}

func TestSortPRsOldestToNewestBreaksCreatedAtTiesByUpdatedAt(t *testing.T) {
	now := time.Now()
	sameCreatedAt := now.Add(-24 * time.Hour)
	updatedLater := testPRView(1, sameCreatedAt, now.Add(-1*time.Hour))
	updatedEarlier := testPRView(2, sameCreatedAt, now.Add(-2*time.Hour))

	result := prview.SortPRsOldestToNewest(
		[]prview.PR{updatedLater, updatedEarlier},
	)

	if result[0].GetNumber() != 2 || result[1].GetNumber() != 1 {
		t.Errorf(
			"expected PR #2 (updated earlier) before PR #1 (updated later), got order [#%d, #%d]",
			result[0].GetNumber(), result[1].GetNumber(),
		)
	}
}

func testPRInRepository(number int, repository models.Repository) prview.PR {
	pr := testPR(number, time.Time{}, time.Time{})
	pr.Repository = repository
	return prview.PR{PR: &pr}
}

func TestGroupPRsByRepositories(t *testing.T) {
	repoA := models.Repository{Owner: "org", Name: "alpha"}
	repoB := models.Repository{Owner: "org", Name: "beta"}
	repoC := models.Repository{Owner: "another-org", Name: "gamma"}

	tests := []struct {
		name                string
		prs                 []prview.PR
		expectedRepos       []models.Repository
		expectedPRNumbersBy map[string][]int
	}{
		{
			name:                "no PRs",
			prs:                 []prview.PR{},
			expectedRepos:       []models.Repository{},
			expectedPRNumbersBy: map[string][]int{},
		},
		{
			name: "single repository keeps every PR in one group",
			prs: []prview.PR{
				testPRInRepository(1, repoA),
				testPRInRepository(2, repoA),
			},
			expectedRepos:       []models.Repository{repoA},
			expectedPRNumbersBy: map[string][]int{"org/alpha": {1, 2}},
		},
		{
			name: "multiple repositories are ordered alphabetically by path, PRs keep input order",
			prs: []prview.PR{
				testPRInRepository(4, repoB),
				testPRInRepository(2, repoC),
				testPRInRepository(3, repoA),
				testPRInRepository(1, repoB),
			},
			expectedRepos: []models.Repository{repoC, repoA, repoB}, // another-org/gamma, org/alpha, org/beta
			expectedPRNumbersBy: map[string][]int{
				"another-org/gamma": {2},
				"org/alpha":         {3},
				// Not ascending by number, so an ascending within-bucket sort fails here; the
				// single-repository case catches a descending one.
				"org/beta": {4, 1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groups := prview.GroupPRsByRepositories(tt.prs)

			if len(groups) != len(tt.expectedRepos) {
				t.Fatalf("expected %d groups, got %d", len(tt.expectedRepos), len(groups))
			}
			for i, expectedRepo := range tt.expectedRepos {
				if groups[i].Repository != expectedRepo {
					t.Errorf("expected group %d to be %v, got %v", i, expectedRepo, groups[i].Repository)
				}
				expectedNumbers := tt.expectedPRNumbersBy[expectedRepo.GetPath()]
				numbers := utilities.Map(groups[i].PRs, func(pr prview.PR) int { return pr.GetNumber() })
				if !slices.Equal(numbers, expectedNumbers) {
					t.Errorf(
						"expected PR numbers %v for %s, got %v",
						expectedNumbers, expectedRepo.GetPath(), numbers,
					)
				}
			}
		})
	}
}

func TestGroupPRsByRepositoriesInGivenOrder(t *testing.T) {
	repoA := models.Repository{Owner: "org", Name: "alpha"}
	repoB := models.Repository{Owner: "org", Name: "beta"}
	repoC := models.Repository{Owner: "another-org", Name: "gamma"}

	tests := []struct {
		name                string
		prs                 []prview.PR
		expectedRepos       []models.Repository
		expectedPRNumbersBy map[string][]int
	}{
		{
			name:                "no PRs",
			prs:                 []prview.PR{},
			expectedRepos:       []models.Repository{},
			expectedPRNumbersBy: map[string][]int{},
		},
		{
			name: "single repository keeps every PR in one group",
			prs: []prview.PR{
				testPRInRepository(1, repoA),
				testPRInRepository(2, repoA),
			},
			expectedRepos:       []models.Repository{repoA},
			expectedPRNumbersBy: map[string][]int{"org/alpha": {1, 2}},
		},
		{
			// Alphabetical order would be another-org/gamma, org/alpha, org/beta, so this case
			// fails against alphabetical bucket ordering.
			name: "multiple repositories are ordered by their first PR in the given list",
			prs: []prview.PR{
				testPRInRepository(4, repoB),
				testPRInRepository(2, repoC),
				testPRInRepository(3, repoA),
				testPRInRepository(1, repoB),
			},
			expectedRepos: []models.Repository{repoB, repoC, repoA},
			expectedPRNumbersBy: map[string][]int{
				// Not ascending by number, so an ascending within-bucket sort fails here; the
				// single-repository case catches a descending one.
				"org/beta":          {4, 1},
				"another-org/gamma": {2},
				"org/alpha":         {3},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groups := prview.GroupPRsByRepositoriesInGivenOrder(tt.prs)

			if len(groups) != len(tt.expectedRepos) {
				t.Fatalf("expected %d groups, got %d", len(tt.expectedRepos), len(groups))
			}
			for i, expectedRepo := range tt.expectedRepos {
				if groups[i].Repository != expectedRepo {
					t.Errorf("expected group %d to be %v, got %v", i, expectedRepo, groups[i].Repository)
				}
				expectedNumbers := tt.expectedPRNumbersBy[expectedRepo.GetPath()]
				numbers := utilities.Map(groups[i].PRs, func(pr prview.PR) int { return pr.GetNumber() })
				if !slices.Equal(numbers, expectedNumbers) {
					t.Errorf(
						"expected PR numbers %v for %s, got %v",
						expectedNumbers, expectedRepo.GetPath(), numbers,
					)
				}
			}
		})
	}
}

func testCollaborator(name string) prview.Collaborator {
	return prview.Collaborator{Collaborator: &githubclient.Collaborator{Login: name, Name: name}}
}

func TestGetReviewersTextSegments(t *testing.T) {
	alice := testCollaborator("Alice")
	bob := testCollaborator("Bob")
	carol := testCollaborator("Carol")

	tests := []struct {
		name       string
		approvers  []prview.Collaborator
		commenters []prview.Collaborator
		expected   []string
	}{
		{
			name:     "no reviewers",
			expected: nil,
		},
		{
			name:      "approvers only",
			approvers: []prview.Collaborator{alice, bob},
			expected:  []string{" (✅ ", "Alice", ", ", "Bob", ")"},
		},
		{
			name:       "commenters only",
			commenters: []prview.Collaborator{carol},
			expected:   []string{" (💬 ", "Carol", ")"},
		},
		{
			name:       "approvers and commenters",
			approvers:  []prview.Collaborator{alice},
			commenters: []prview.Collaborator{bob, carol},
			expected:   []string{" (✅ ", "Alice", " / 💬 ", "Bob", ", ", "Carol", ")"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			segments := prview.GetReviewersTextSegments(tt.approvers, tt.commenters)
			if !slices.Equal(segments, tt.expected) {
				t.Errorf("expected segments %q, got %q", tt.expected, segments)
			}
		})
	}
}

func TestGetPRAgeDisplayText(t *testing.T) {
	tests := []struct {
		name     string
		isOldPR  bool
		expected string
	}{
		{name: "PR under the old-PR threshold", isOldPR: false, expected: "3 days ago"},
		{name: "PR past the old-PR threshold", isOldPR: true, expected: "3 days old"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := testPR(1, time.Now().Add(-72*time.Hour), time.Time{})
			prView := prview.PR{PR: &pr, IsOldPR: tt.isOldPR}

			if prView.GetPRAgeDisplayText() != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, prView.GetPRAgeDisplayText())
			}
		})
	}
}

func testPRWithUpdatedAt(number int, updatedAt time.Time) prview.PR {
	pr := testPR(number, time.Time{}, updatedAt)
	return prview.PR{PR: &pr}
}

func timePointer(t time.Time) *time.Time {
	return &t
}

func TestGetActivityText(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name      string
		updatedAt time.Time
		expected  string
	}{
		{name: "unknown activity", updatedAt: time.Time{}, expected: ""},
		{
			name:      "minutes ago",
			updatedAt: now.Add(-30 * time.Minute),
			expected:  "updated 30 minutes ago",
		},
		{
			name:      "hours ago",
			updatedAt: now.Add(-5 * time.Hour),
			expected:  "updated 5 hours ago",
		},
		{
			name:      "just under the day cutover",
			updatedAt: now.Add(-23*time.Hour - 30*time.Minute),
			expected:  "updated 24 hours ago",
		},
		{
			name:      "one hour under the day cutover",
			updatedAt: now.Add(-23 * time.Hour),
			expected:  "updated 23 hours ago",
		},
		{
			name:      "exactly at the day cutover",
			updatedAt: now.Add(-24 * time.Hour),
			expected:  "idle 1 day",
		},
		{
			name:      "just past the day cutover",
			updatedAt: now.Add(-24*time.Hour - 30*time.Minute),
			expected:  "idle 1 day",
		},
		{
			name:      "idle for days",
			updatedAt: now.Add(-72 * time.Hour),
			expected:  "idle 3 days",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := testPRWithUpdatedAt(1, tt.updatedAt)
			if got := pr.GetActivityText(); got != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, got)
			}
		})
	}
}

func testMergedPRWithNumber(number int, mergedAt *time.Time) prview.PR {
	pr := testPR(number, time.Time{}, time.Time{})
	pr.MergedAt = mergedAt
	return prview.PR{PR: &pr}
}

func TestGetMergedText(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		mergedAt *time.Time
		expected string
	}{
		{name: "never merged", mergedAt: nil, expected: ""},
		{
			name:     "minutes ago",
			mergedAt: timePointer(now.Add(-30 * time.Minute)),
			expected: "merged 30 minutes ago",
		},
		{
			name:     "hours ago",
			mergedAt: timePointer(now.Add(-2 * time.Hour)),
			expected: "merged 2 hours ago",
		},
		{
			name:     "days ago",
			mergedAt: timePointer(now.Add(-72 * time.Hour)),
			expected: "merged 3 days ago",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := testMergedPRWithNumber(1, tt.mergedAt).GetMergedText(); got != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, got)
			}
		})
	}
}

func TestIsActiveAsOf(t *testing.T) {
	asOf := time.Now()
	tests := []struct {
		name      string
		updatedAt time.Time
		expected  bool
	}{
		{name: "unknown activity is active", updatedAt: time.Time{}, expected: true},
		{
			name:      "just under the threshold",
			updatedAt: asOf.Add(-23 * time.Hour),
			expected:  true,
		},
		{
			name:      "exactly at the threshold",
			updatedAt: asOf.Add(-24 * time.Hour),
			expected:  true,
		},
		{
			name:      "past the threshold",
			updatedAt: asOf.Add(-25 * time.Hour),
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := testPRWithUpdatedAt(1, tt.updatedAt)
			if got := pr.IsActiveAsOf(asOf, prview.RecentActivityThreshold); got != tt.expected {
				t.Errorf("expected %t, got %t", tt.expected, got)
			}
		})
	}
}

func TestIsRecentlyUpdated(t *testing.T) {
	tests := []struct {
		name      string
		updatedAt time.Time
		expected  bool
	}{
		{name: "recent activity", updatedAt: time.Now().Add(-23 * time.Hour), expected: true},
		{name: "stale activity", updatedAt: time.Now().Add(-25 * time.Hour), expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := testPRWithUpdatedAt(1, tt.updatedAt)
			if got := pr.IsRecentlyUpdated(); got != tt.expected {
				t.Errorf("expected %t, got %t", tt.expected, got)
			}
		})
	}
}

func TestIsOpen(t *testing.T) {
	tests := []struct {
		name     string
		draft    bool
		expected bool
	}{
		{name: "not a draft is open", draft: false, expected: true},
		{name: "draft is not open", draft: true, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := testDraftPR(tt.draft)
			if got := pr.IsOpen(); got != tt.expected {
				t.Errorf("expected %t, got %t", tt.expected, got)
			}
		})
	}
}

func TestIsDraft(t *testing.T) {
	tests := []struct {
		name     string
		draft    bool
		expected bool
	}{
		{name: "not a draft", draft: false, expected: false},
		{name: "draft", draft: true, expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := testDraftPR(tt.draft)
			if got := pr.IsDraft(); got != tt.expected {
				t.Errorf("expected %t, got %t", tt.expected, got)
			}
		})
	}
}

func testDraftPR(draft bool) prview.PR {
	pr := testPR(1, time.Time{}, time.Time{})
	pr.Draft = draft
	return prview.PR{PR: &pr}
}

func TestLastActivityAt(t *testing.T) {
	updatedAt := time.Now().Add(-3 * time.Hour)

	t.Run("known activity is returned as a pointer to it", func(t *testing.T) {
		pr := testPRWithUpdatedAt(1, updatedAt)
		got := pr.LastActivityAt()
		if got == nil || !got.Equal(updatedAt) {
			t.Errorf("expected %v, got %v", updatedAt, got)
		}
	})

	t.Run("unknown activity is nil, not the zero time", func(t *testing.T) {
		pr := testPRWithUpdatedAt(1, time.Time{})
		if got := pr.LastActivityAt(); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})
}

func TestSortPRsNewestFirst(t *testing.T) {
	now := time.Now()
	unknownMerge := testMergedPRWithNumber(1, nil)
	oldestMerge := testMergedPRWithNumber(2, timePointer(now.Add(-72*time.Hour)))
	newestMerge := testMergedPRWithNumber(3, timePointer(now.Add(-1*time.Hour)))
	alsoUnknownMerge := testMergedPRWithNumber(4, nil)
	middleMerge := testMergedPRWithNumber(5, timePointer(now.Add(-24*time.Hour)))

	given := []prview.PR{unknownMerge, oldestMerge, newestMerge, alsoUnknownMerge, middleMerge}

	sorted := prview.SortPRsNewestFirst(given, func(pr prview.PR) *time.Time {
		return pr.MergedAt
	})

	want := []int{3, 5, 2, 1, 4}
	for i, number := range want {
		if sorted[i].GetNumber() != number {
			t.Errorf("expected PR at position %d to be #%d, got #%d", i, number, sorted[i].GetNumber())
		}
	}

	givenNumbers := utilities.Map(given, func(pr prview.PR) int { return pr.GetNumber() })
	if !slices.Equal(givenNumbers, []int{1, 2, 3, 4, 5}) {
		t.Errorf("expected the given slice to keep its order, got %v", givenNumbers)
	}
}

func nextActionPR(approverLogins []string, prFlags githubclient.PR) prview.PR {
	prFlags.PullRequest = &githubclient.PullRequest{
		Author: githubclient.Collaborator{Login: "author"},
	}
	return prview.PR{
		PR: &prFlags,
		Approvers: utilities.Map(approverLogins, func(login string) prview.Collaborator {
			return prview.NewCollaborator(githubclient.Collaborator{Login: login}, "")
		}),
	}
}

// The checks are ordered, so each case that an earlier check claims has to stay claimed: a
// commented-then-approved PR is ready to merge, and a conflict only ever demotes.
func TestGetNextAction(t *testing.T) {
	tests := []struct {
		name           string
		approverLogins []string
		prFlags        githubclient.PR
		expected       prview.PRNextAction
	}{
		{
			name:           "approved with nothing outstanding",
			approverLogins: []string{"bob"},
			expected:       prview.NextActionReadyToMerge,
		},
		{
			name:           "approved, and the author has not answered a thread",
			approverLogins: []string{"bob"},
			prFlags:        githubclient.PR{HasThreadWaitingForAuthor: true},
			expected:       prview.NextActionWaitingForAuthor,
		},
		{
			// A conflict cannot be merged, so it demotes an approved PR to its author.
			name:           "approved but conflicting",
			approverLogins: []string{"carol"},
			prFlags:        githubclient.PR{Conflicting: true},
			expected:       prview.NextActionWaitingForAuthor,
		},
		{
			name:           "approved twice but conflicting",
			approverLogins: []string{"bob", "carol"},
			prFlags:        githubclient.PR{Conflicting: true},
			expected:       prview.NextActionWaitingForAuthor,
		},
		{
			name:     "a reviewer requested changes",
			prFlags:  githubclient.PR{HasNonApprovingReview: true},
			expected: prview.NextActionWaitingForAuthor,
		},
		{
			name:     "an unanswered thread without any review",
			prFlags:  githubclient.PR{HasThreadWaitingForAuthor: true},
			expected: prview.NextActionWaitingForAuthor,
		},
		{
			name:     "nobody has looked at it yet",
			expected: prview.NextActionWaitingForReview,
		},
		{
			// An unreviewed conflict stays in the review queue: reviewing around a rebase is
			// not wasted work.
			name:     "conflicting and nothing else",
			prFlags:  githubclient.PR{Conflicting: true},
			expected: prview.NextActionWaitingForReview,
		},
		{
			// The reviewer commented and then approved, which leaves both signals set.
			name:           "approved by the reviewer who commented",
			approverLogins: []string{"dave"},
			prFlags:        githubclient.PR{HasNonApprovingReview: true},
			expected:       prview.NextActionReadyToMerge,
		},
		{
			name:           "approved, commented on, and a thread left unanswered",
			approverLogins: []string{"erin", "bob"},
			prFlags: githubclient.PR{
				HasNonApprovingReview: true, HasThreadWaitingForAuthor: true,
			},
			expected: prview.NextActionWaitingForAuthor,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if nextAction := nextActionPR(tt.approverLogins, tt.prFlags).GetNextAction(); nextAction != tt.expected {
				t.Errorf("GetNextAction() = %q, expected %q", nextAction, tt.expected)
			}
		})
	}
}

// A PR without its fetched half carries no signal at all. It keeps its place in the review
// queue rather than panicking a canvas render or claiming a merge nobody approved.
func TestGetNextActionOfPRWithoutFetchedData(t *testing.T) {
	if nextAction := (prview.PR{}).GetNextAction(); nextAction != prview.NextActionWaitingForReview {
		t.Errorf("GetNextAction() = %q, expected %q", nextAction, prview.NextActionWaitingForReview)
	}
}
