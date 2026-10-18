// Package mergestats computes the merge stats: how many PRs merged in a window, and the median
// weekday time they took to a first review and to merge.
package mergestats

import (
	"slices"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/prview"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

// See mergestats.spec.md § Behaviour.
type MergeStats struct {
	MergedCount             int            `json:"mergedCount"`
	MedianTimeToFirstReview *time.Duration `json:"medianTimeToFirstReview,omitempty"`
	MedianTimeToMerge       *time.Duration `json:"medianTimeToMerge,omitempty"`
	MergedSince             time.Time      `json:"mergedSince"`
	ComputedAt              time.Time      `json:"computedAt"`
}

func FromMergedPRs(prs []githubclient.PR, mergedSince, computedAt time.Time) MergeStats {
	timedPRs := utilities.Filter(prs, isTimedPR)
	reviewedPRs := utilities.Filter(timedPRs, func(pr githubclient.PR) bool {
		return pr.FirstReviewAt != nil
	})
	return MergeStats{
		MergedCount:             len(prs),
		MedianTimeToFirstReview: median(utilities.Map(reviewedPRs, timeToFirstReview)),
		MedianTimeToMerge:       median(utilities.Map(timedPRs, timeToMerge)),
		MergedSince:             mergedSince,
		ComputedAt:              computedAt,
	}
}

// Without complete reviewer info, the review and ready-for-review times may be missing. And
// WeekdayTimeBetween walks from year 1 on a zero start. See mergestats.spec.md § Oddities
func isTimedPR(pr githubclient.PR) bool {
	return pr.HasCompleteReviewerInfo && !pr.ReadyForReviewAt().IsZero() && pr.GetMergedAt() != nil
}

// A review before the PR turned ready for review counts as 0. See mergestats.spec.md § Behaviour
func timeToFirstReview(pr githubclient.PR) time.Duration {
	return prview.WeekdayTimeBetween(pr.ReadyForReviewAt(), *pr.FirstReviewAt)
}

func timeToMerge(pr githubclient.PR) time.Duration {
	return prview.WeekdayTimeBetween(pr.ReadyForReviewAt(), *pr.GetMergedAt())
}

func median(durations []time.Duration) *time.Duration {
	if len(durations) == 0 {
		return nil
	}
	sorted := slices.Sorted(slices.Values(durations))
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return &sorted[middle]
	}
	mean := (sorted[middle-1] + sorted[middle]) / 2
	return &mean
}
