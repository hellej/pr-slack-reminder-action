// Package mergestats computes the weekly merge stats: how many PRs merged in a window, and the
// median weekday time they took to a first review and to merge.
package mergestats

import (
	"math"
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
	ComputedAt              time.Time      `json:"computedAt"`
}

func FromMergedPRs(prs []githubclient.PR, computedAt time.Time) MergeStats {
	timedPRs := utilities.Filter(prs, hasReadyForReviewAndMergeTimes)
	reviewedPRs := utilities.Filter(timedPRs, func(pr githubclient.PR) bool {
		return pr.FirstReviewAt != nil
	})
	return MergeStats{
		MergedCount:             len(prs),
		MedianTimeToFirstReview: median(utilities.Map(reviewedPRs, timeToFirstReview)),
		MedianTimeToMerge:       median(utilities.Map(timedPRs, timeToMerge)),
		ComputedAt:              computedAt,
	}
}

// WeekdayTimeBetween walks from year 1 on a zero start, so a PR without a known start is
// left out of both medians.
func hasReadyForReviewAndMergeTimes(pr githubclient.PR) bool {
	return !pr.ReadyForReviewAt().IsZero() && pr.GetMergedAt() != nil
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

// See mergestats.spec.md § Behaviour.
func PercentChanges(shown, weekBefore MergeStats) (firstReviewChange, mergeChange *int) {
	return percentChange(shown.MedianTimeToFirstReview, weekBefore.MedianTimeToFirstReview),
		percentChange(shown.MedianTimeToMerge, weekBefore.MedianTimeToMerge)
}

func percentChange(shown, weekBefore *time.Duration) *int {
	if shown == nil || weekBefore == nil || *weekBefore == 0 {
		return nil
	}
	change := int(math.Round(100 * float64(*shown-*weekBefore) / float64(*weekBefore)))
	return &change
}
