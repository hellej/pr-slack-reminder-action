package mergestats_test

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/mergestats"
)

var computedAt = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)

// 2026-10-06 is a Tuesday, 2026-10-09 a Friday, 2026-10-12 a Monday.
func at(day, hour int) time.Time {
	return time.Date(2026, time.October, day, hour, 0, 0, 0, time.UTC)
}

type mergedPROptions struct {
	createdAt     time.Time
	readiedAt     *time.Time
	firstReviewAt *time.Time
	mergedAt      time.Time
	authorLogin   string
}

func mergedPR(options mergedPROptions) githubclient.PR {
	return githubclient.PR{
		PullRequest: &githubclient.PullRequest{
			CreatedAt: options.createdAt,
			Merged:    true,
			MergedAt:  &options.mergedAt,
			Author:    githubclient.Collaborator{Login: options.authorLogin},
		},
		FirstReadyForReviewEventAt: options.readiedAt,
		FirstReviewAt:              options.firstReviewAt,
	}
}

func durationPtr(duration time.Duration) *time.Duration {
	return &duration
}

func timePtr(t time.Time) *time.Time {
	return &t
}

func TestFromMergedPRs(t *testing.T) {
	tests := []struct {
		name     string
		prs      []githubclient.PR
		expected mergestats.MergeStats
	}{
		{
			name:     "nothing merged",
			prs:      nil,
			expected: mergestats.MergeStats{ComputedAt: computedAt},
		},
		{
			name: "odd count, out of order",
			prs: []githubclient.PR{
				mergedPR(mergedPROptions{createdAt: at(6, 9), firstReviewAt: timePtr(at(6, 14)), mergedAt: at(6, 17)}),
				mergedPR(mergedPROptions{createdAt: at(6, 9), firstReviewAt: timePtr(at(6, 10)), mergedAt: at(6, 11)}),
				mergedPR(mergedPROptions{createdAt: at(6, 9), firstReviewAt: timePtr(at(6, 12)), mergedAt: at(6, 21)}),
			},
			expected: mergestats.MergeStats{
				MergedCount:             3,
				MedianTimeToFirstReview: durationPtr(3 * time.Hour),
				MedianTimeToMerge:       durationPtr(8 * time.Hour),
				ComputedAt:              computedAt,
			},
		},
		{
			name: "even count takes the mean of the two middle values",
			prs: []githubclient.PR{
				mergedPR(mergedPROptions{createdAt: at(6, 0), firstReviewAt: timePtr(at(6, 8)), mergedAt: at(6, 10)}),
				mergedPR(mergedPROptions{createdAt: at(6, 0), firstReviewAt: timePtr(at(6, 1)), mergedAt: at(6, 1)}),
				mergedPR(mergedPROptions{createdAt: at(6, 0), firstReviewAt: timePtr(at(6, 4)), mergedAt: at(6, 20)}),
				mergedPR(mergedPROptions{createdAt: at(6, 0), firstReviewAt: timePtr(at(6, 2)), mergedAt: at(6, 3)}),
			},
			expected: mergestats.MergeStats{
				MergedCount:             4,
				MedianTimeToFirstReview: durationPtr(3 * time.Hour),
				MedianTimeToMerge:       durationPtr(6*time.Hour + 30*time.Minute),
				ComputedAt:              computedAt,
			},
		},
		{
			// Wall-clock time would read 49 and 74 hours.
			name: "readied on Friday, reviewed and merged on Monday, counts weekday time",
			prs: []githubclient.PR{
				mergedPR(mergedPROptions{createdAt: at(9, 23), firstReviewAt: timePtr(at(12, 0)), mergedAt: at(13, 1)}),
			},
			expected: mergestats.MergeStats{
				MergedCount:             1,
				MedianTimeToFirstReview: durationPtr(time.Hour),
				MedianTimeToMerge:       durationPtr(26 * time.Hour),
				ComputedAt:              computedAt,
			},
		},
		{
			name: "counts from the ready-for-review time, and a review before it as 0",
			prs: []githubclient.PR{
				mergedPR(mergedPROptions{
					createdAt: at(5, 9), readiedAt: timePtr(at(6, 9)),
					firstReviewAt: timePtr(at(5, 12)), mergedAt: at(6, 13),
				}),
			},
			expected: mergestats.MergeStats{
				MergedCount:             1,
				MedianTimeToFirstReview: durationPtr(0),
				MedianTimeToMerge:       durationPtr(4 * time.Hour),
				ComputedAt:              computedAt,
			},
		},
		{
			name: "PRs without a first review count toward the merge median only, a bot's PR included",
			prs: []githubclient.PR{
				mergedPR(mergedPROptions{createdAt: at(6, 9), mergedAt: at(6, 10), authorLogin: "dependabot[bot]"}),
				mergedPR(mergedPROptions{createdAt: at(6, 9), firstReviewAt: timePtr(at(6, 11)), mergedAt: at(6, 12)}),
				mergedPR(mergedPROptions{createdAt: at(6, 9), mergedAt: at(6, 14), authorLogin: "dependabot[bot]"}),
			},
			expected: mergestats.MergeStats{
				MergedCount:             3,
				MedianTimeToFirstReview: durationPtr(2 * time.Hour),
				MedianTimeToMerge:       durationPtr(3 * time.Hour),
				ComputedAt:              computedAt,
			},
		},
		{
			name: "no PR had a first review",
			prs: []githubclient.PR{
				mergedPR(mergedPROptions{createdAt: at(6, 9), mergedAt: at(6, 10)}),
				mergedPR(mergedPROptions{createdAt: at(6, 9), mergedAt: at(6, 12)}),
			},
			expected: mergestats.MergeStats{
				MergedCount:       2,
				MedianTimeToMerge: durationPtr(2 * time.Hour),
				ComputedAt:        computedAt,
			},
		},
		{
			name: "a zero ready-for-review time counts as merged, in neither median",
			prs: []githubclient.PR{
				mergedPR(mergedPROptions{firstReviewAt: timePtr(at(6, 10)), mergedAt: at(6, 12)}),
				mergedPR(mergedPROptions{createdAt: at(6, 9), firstReviewAt: timePtr(at(6, 10)), mergedAt: at(6, 11)}),
			},
			expected: mergestats.MergeStats{
				MergedCount:             2,
				MedianTimeToFirstReview: durationPtr(time.Hour),
				MedianTimeToMerge:       durationPtr(2 * time.Hour),
				ComputedAt:              computedAt,
			},
		},
		{
			name: "a PR without a merge time counts as merged, in neither median",
			prs: []githubclient.PR{
				{PullRequest: &githubclient.PullRequest{CreatedAt: at(6, 9), Merged: true}, FirstReviewAt: timePtr(at(6, 10))},
			},
			expected: mergestats.MergeStats{MergedCount: 1, ComputedAt: computedAt},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergestats.FromMergedPRs(tt.prs, computedAt)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("expected %s, got %s", describe(tt.expected), describe(got))
			}
		})
	}
}

func describe(stats mergestats.MergeStats) string {
	durationText := func(duration *time.Duration) string {
		if duration == nil {
			return "nil"
		}
		return duration.String()
	}
	return fmt.Sprintf(
		"{merged %d, first review %s, merge %s, computed at %s}",
		stats.MergedCount, durationText(stats.MedianTimeToFirstReview),
		durationText(stats.MedianTimeToMerge), stats.ComputedAt,
	)
}

func TestPercentChanges(t *testing.T) {
	tests := []struct {
		name                      string
		shown                     mergestats.MergeStats
		weekBefore                mergestats.MergeStats
		expectedFirstReviewChange *int
		expectedMergeChange       *int
	}{
		{
			name:                      "a rise and a drop",
			shown:                     mergestats.MergeStats{MedianTimeToFirstReview: durationPtr(23 * time.Hour), MedianTimeToMerge: durationPtr(34 * time.Hour)},
			weekBefore:                mergestats.MergeStats{MedianTimeToFirstReview: durationPtr(20 * time.Hour), MedianTimeToMerge: durationPtr(40 * time.Hour)},
			expectedFirstReviewChange: new(15),
			expectedMergeChange:       new(-15),
		},
		{
			name:                      "half values round away from zero",
			shown:                     mergestats.MergeStats{MedianTimeToFirstReview: durationPtr(9 * time.Hour), MedianTimeToMerge: durationPtr(7 * time.Hour)},
			weekBefore:                mergestats.MergeStats{MedianTimeToFirstReview: durationPtr(8 * time.Hour), MedianTimeToMerge: durationPtr(8 * time.Hour)},
			expectedFirstReviewChange: new(13),
			expectedMergeChange:       new(-13),
		},
		{
			name:                      "from the stored durations, not the rounded text: both read 1.0 days",
			shown:                     mergestats.MergeStats{MedianTimeToMerge: durationPtr(24*time.Hour + 72*time.Minute)},
			weekBefore:                mergestats.MergeStats{MedianTimeToMerge: durationPtr(24 * time.Hour)},
			expectedFirstReviewChange: nil,
			expectedMergeChange:       new(5),
		},
		{
			name:                      "small changes round to 0",
			shown:                     mergestats.MergeStats{MedianTimeToFirstReview: durationPtr(999 * time.Hour), MedianTimeToMerge: durationPtr(1001 * time.Hour)},
			weekBefore:                mergestats.MergeStats{MedianTimeToFirstReview: durationPtr(1000 * time.Hour), MedianTimeToMerge: durationPtr(1000 * time.Hour)},
			expectedFirstReviewChange: new(0),
			expectedMergeChange:       new(0),
		},
		{
			name:                      "a week-before value of 0",
			shown:                     mergestats.MergeStats{MedianTimeToFirstReview: durationPtr(time.Hour), MedianTimeToMerge: durationPtr(0)},
			weekBefore:                mergestats.MergeStats{MedianTimeToFirstReview: durationPtr(0), MedianTimeToMerge: durationPtr(0)},
			expectedFirstReviewChange: nil,
			expectedMergeChange:       nil,
		},
		{
			name:                      "a shown value of 0 is a drop of 100",
			shown:                     mergestats.MergeStats{MedianTimeToFirstReview: durationPtr(0)},
			weekBefore:                mergestats.MergeStats{MedianTimeToFirstReview: durationPtr(4 * time.Hour)},
			expectedFirstReviewChange: new(-100),
			expectedMergeChange:       nil,
		},
		{
			name:                      "missing this week",
			shown:                     mergestats.MergeStats{MedianTimeToMerge: durationPtr(2 * time.Hour)},
			weekBefore:                mergestats.MergeStats{MedianTimeToFirstReview: durationPtr(time.Hour), MedianTimeToMerge: durationPtr(4 * time.Hour)},
			expectedFirstReviewChange: nil,
			expectedMergeChange:       new(-50),
		},
		{
			name:                      "missing the week before",
			shown:                     mergestats.MergeStats{MedianTimeToFirstReview: durationPtr(3 * time.Hour), MedianTimeToMerge: durationPtr(2 * time.Hour)},
			weekBefore:                mergestats.MergeStats{MedianTimeToFirstReview: durationPtr(time.Hour)},
			expectedFirstReviewChange: new(200),
			expectedMergeChange:       nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			firstReviewChange, mergeChange := mergestats.PercentChanges(tt.shown, tt.weekBefore)
			if !reflect.DeepEqual(firstReviewChange, tt.expectedFirstReviewChange) {
				t.Errorf("Expected the first review change %s, got %s", percentText(tt.expectedFirstReviewChange), percentText(firstReviewChange))
			}
			if !reflect.DeepEqual(mergeChange, tt.expectedMergeChange) {
				t.Errorf("Expected the merge change %s, got %s", percentText(tt.expectedMergeChange), percentText(mergeChange))
			}
		})
	}
}

func percentText(change *int) string {
	if change == nil {
		return "nil"
	}
	return fmt.Sprintf("%d %%", *change)
}
