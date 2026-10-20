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

var mergedSince = time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)

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
	// Left false, the PR came with complete reviewer info
	incompleteReviewerInfo bool
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
		HasCompleteReviewerInfo:    !options.incompleteReviewerInfo,
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
				{
					PullRequest:   &githubclient.PullRequest{CreatedAt: at(6, 9), Merged: true},
					FirstReviewAt: timePtr(at(6, 10)), HasCompleteReviewerInfo: true,
				},
			},
			expected: mergestats.MergeStats{MergedCount: 1, ComputedAt: computedAt},
		},
		{
			// Counting it would give 3 values, medians 3 hours to first review and 4 to merge.
			name: "a PR without complete reviewer info counts as merged, in neither median",
			prs: []githubclient.PR{
				mergedPR(mergedPROptions{createdAt: at(6, 9), firstReviewAt: timePtr(at(6, 10)), mergedAt: at(6, 11)}),
				mergedPR(mergedPROptions{
					createdAt: at(6, 9), firstReviewAt: timePtr(at(6, 18)), mergedAt: at(6, 21),
					incompleteReviewerInfo: true,
				}),
				mergedPR(mergedPROptions{createdAt: at(6, 9), firstReviewAt: timePtr(at(6, 12)), mergedAt: at(6, 13)}),
			},
			expected: mergestats.MergeStats{
				MergedCount:             3,
				MedianTimeToFirstReview: durationPtr(2 * time.Hour),
				MedianTimeToMerge:       durationPtr(3 * time.Hour),
				ComputedAt:              computedAt,
			},
		},
		{
			name: "every PR without complete reviewer info gives the count only",
			prs: []githubclient.PR{
				mergedPR(mergedPROptions{createdAt: at(6, 9), mergedAt: at(6, 11), incompleteReviewerInfo: true}),
				mergedPR(mergedPROptions{createdAt: at(6, 9), mergedAt: at(6, 13), incompleteReviewerInfo: true}),
			},
			expected: mergestats.MergeStats{MergedCount: 2, ComputedAt: computedAt},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expected := tt.expected
			expected.MergedSince = mergedSince
			got := mergestats.FromMergedPRs(tt.prs, mergedSince, computedAt)
			if !reflect.DeepEqual(got, expected) {
				t.Errorf("expected %s, got %s", describe(expected), describe(got))
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
		"{merged %d, first review %s, merge %s, merged since %s, computed at %s}",
		stats.MergedCount, durationText(stats.MedianTimeToFirstReview),
		durationText(stats.MedianTimeToMerge), stats.MergedSince, stats.ComputedAt,
	)
}
