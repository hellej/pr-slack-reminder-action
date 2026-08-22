package prparser_test

import (
	"slices"
	"testing"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/prparser"
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

func TestParsePRsSortsOldestCreatedFirst(t *testing.T) {
	now := time.Now()
	oldest := testPR(1, now.Add(-48*time.Hour), now.Add(-48*time.Hour))
	middle := testPR(2, now.Add(-24*time.Hour), now.Add(-24*time.Hour))
	newest := testPR(3, now.Add(-1*time.Hour), now.Add(-1*time.Hour))

	result := prparser.ParsePRs(
		[]githubclient.PR{newest, oldest, middle},
		config.ContentInputs{},
	)

	want := []int{1, 2, 3}
	for i, number := range want {
		if result[i].GetNumber() != number {
			t.Errorf("expected PR at position %d to be #%d, got #%d", i, number, result[i].GetNumber())
		}
	}
}

func TestParsePRsBreaksCreatedAtTiesByUpdatedAtOldestFirst(t *testing.T) {
	now := time.Now()
	sameCreatedAt := now.Add(-24 * time.Hour)
	updatedLater := testPR(1, sameCreatedAt, now.Add(-1*time.Hour))
	updatedEarlier := testPR(2, sameCreatedAt, now.Add(-2*time.Hour))

	result := prparser.ParsePRs(
		[]githubclient.PR{updatedLater, updatedEarlier},
		config.ContentInputs{},
	)

	if result[0].GetNumber() != 2 || result[1].GetNumber() != 1 {
		t.Errorf(
			"expected PR #2 (updated earlier) before PR #1 (updated later), got order [#%d, #%d]",
			result[0].GetNumber(), result[1].GetNumber(),
		)
	}
}

func testPRInRepository(number int, repository models.Repository) prparser.PR {
	pr := testPR(number, time.Time{}, time.Time{})
	pr.Repository = repository
	return prparser.PR{PR: &pr}
}

func TestGroupPRsByRepositories(t *testing.T) {
	repoA := models.Repository{Owner: "org", Name: "alpha"}
	repoB := models.Repository{Owner: "org", Name: "beta"}
	repoC := models.Repository{Owner: "another-org", Name: "gamma"}

	tests := []struct {
		name                string
		prs                 []prparser.PR
		expectedRepos       []models.Repository
		expectedPRNumbersBy map[string][]int
	}{
		{
			name:                "no PRs",
			prs:                 []prparser.PR{},
			expectedRepos:       []models.Repository{},
			expectedPRNumbersBy: map[string][]int{},
		},
		{
			name: "single repository keeps every PR in one group",
			prs: []prparser.PR{
				testPRInRepository(1, repoA),
				testPRInRepository(2, repoA),
			},
			expectedRepos:       []models.Repository{repoA},
			expectedPRNumbersBy: map[string][]int{"org/alpha": {1, 2}},
		},
		{
			name: "multiple repositories are ordered alphabetically by path, PRs keep input order",
			prs: []prparser.PR{
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
			groups := prparser.GroupPRsByRepositories(tt.prs)

			if len(groups) != len(tt.expectedRepos) {
				t.Fatalf("expected %d groups, got %d", len(tt.expectedRepos), len(groups))
			}
			for i, expectedRepo := range tt.expectedRepos {
				if groups[i].Repository != expectedRepo {
					t.Errorf("expected group %d to be %v, got %v", i, expectedRepo, groups[i].Repository)
				}
				expectedNumbers := tt.expectedPRNumbersBy[expectedRepo.GetPath()]
				numbers := utilities.Map(groups[i].PRs, func(pr prparser.PR) int { return pr.GetNumber() })
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

func testCollaborator(name string) prparser.Collaborator {
	return prparser.Collaborator{Collaborator: &githubclient.Collaborator{Login: name, Name: name}}
}

func TestGetReviewersTextSegments(t *testing.T) {
	alice := testCollaborator("Alice")
	bob := testCollaborator("Bob")
	carol := testCollaborator("Carol")

	tests := []struct {
		name       string
		approvers  []prparser.Collaborator
		commenters []prparser.Collaborator
		expected   []string
	}{
		{
			name:     "no reviewers",
			expected: nil,
		},
		{
			name:      "approvers only",
			approvers: []prparser.Collaborator{alice, bob},
			expected:  []string{" (✅ ", "Alice", ", ", "Bob", ")"},
		},
		{
			name:       "commenters only",
			commenters: []prparser.Collaborator{carol},
			expected:   []string{" (💬 ", "Carol", ")"},
		},
		{
			name:       "approvers and commenters",
			approvers:  []prparser.Collaborator{alice},
			commenters: []prparser.Collaborator{bob, carol},
			expected:   []string{" (✅ ", "Alice", " / 💬 ", "Bob", ", ", "Carol", ")"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			segments := prparser.GetReviewersTextSegments(tt.approvers, tt.commenters)
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
			parsed := prparser.PR{PR: &pr, IsOldPR: tt.isOldPR}

			if parsed.GetPRAgeDisplayText() != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, parsed.GetPRAgeDisplayText())
			}
		})
	}
}

func testPRWithLastActivity(number int, lastActivityAt *time.Time) prparser.PR {
	pr := testPR(number, time.Time{}, time.Time{})
	pr.LastActivityAt = lastActivityAt
	return prparser.PR{PR: &pr}
}

func timePointer(t time.Time) *time.Time {
	return &t
}

func TestGetActivityText(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name           string
		lastActivityAt *time.Time
		expected       string
	}{
		{name: "unknown activity", lastActivityAt: nil, expected: ""},
		{
			name:           "minutes ago",
			lastActivityAt: timePointer(now.Add(-30 * time.Minute)),
			expected:       "updated 30 minutes ago",
		},
		{
			name:           "hours ago",
			lastActivityAt: timePointer(now.Add(-5 * time.Hour)),
			expected:       "updated 5 hours ago",
		},
		{
			name:           "just under the day cutover",
			lastActivityAt: timePointer(now.Add(-23*time.Hour - 30*time.Minute)),
			expected:       "updated 24 hours ago",
		},
		{
			name:           "just past the day cutover",
			lastActivityAt: timePointer(now.Add(-24*time.Hour - 30*time.Minute)),
			expected:       "idle 1 days",
		},
		{
			name:           "idle for days",
			lastActivityAt: timePointer(now.Add(-72 * time.Hour)),
			expected:       "idle 3 days",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := testPRWithLastActivity(1, tt.lastActivityAt)
			if got := pr.GetActivityText(); got != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, got)
			}
		})
	}
}

func testMergedPR(mergedAt *time.Time) prparser.PR {
	pr := testPR(1, time.Time{}, time.Time{})
	pr.MergedAt = mergedAt
	return prparser.PR{PR: &pr}
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
			if got := testMergedPR(tt.mergedAt).GetMergedText(); got != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, got)
			}
		})
	}
}

func TestIsIdle(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name           string
		lastActivityAt *time.Time
		expected       bool
	}{
		{name: "unknown activity is not idle", lastActivityAt: nil, expected: false},
		{
			name:           "active within 48 hours",
			lastActivityAt: timePointer(now.Add(-47 * time.Hour)),
			expected:       false,
		},
		{
			name:           "inactive for over 48 hours",
			lastActivityAt: timePointer(now.Add(-49 * time.Hour)),
			expected:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := testPRWithLastActivity(1, tt.lastActivityAt)
			if got := pr.IsIdle(); got != tt.expected {
				t.Errorf("expected %t, got %t", tt.expected, got)
			}
		})
	}
}

func TestSortPRsNewestFirst(t *testing.T) {
	now := time.Now()
	unknownActivity := testPRWithLastActivity(1, nil)
	oldestActivity := testPRWithLastActivity(2, timePointer(now.Add(-72*time.Hour)))
	newestActivity := testPRWithLastActivity(3, timePointer(now.Add(-1*time.Hour)))
	alsoUnknownActivity := testPRWithLastActivity(4, nil)
	middleActivity := testPRWithLastActivity(5, timePointer(now.Add(-24*time.Hour)))

	given := []prparser.PR{unknownActivity, oldestActivity, newestActivity, alsoUnknownActivity, middleActivity}

	sorted := prparser.SortPRsNewestFirst(given, func(pr prparser.PR) *time.Time {
		return pr.LastActivityAt
	})

	want := []int{3, 5, 2, 1, 4}
	for i, number := range want {
		if sorted[i].GetNumber() != number {
			t.Errorf("expected PR at position %d to be #%d, got #%d", i, number, sorted[i].GetNumber())
		}
	}

	givenNumbers := utilities.Map(given, func(pr prparser.PR) int { return pr.GetNumber() })
	if !slices.Equal(givenNumbers, []int{1, 2, 3, 4, 5}) {
		t.Errorf("expected the given slice to keep its order, got %v", givenNumbers)
	}
}
