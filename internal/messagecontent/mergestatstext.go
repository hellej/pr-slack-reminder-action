package messagecontent

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/mergestats"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

type shownMedian struct {
	text          string
	percentChange *int
}

func mergeStatsText(stats, weekBefore *mergestats.MergeStats) string {
	if stats == nil {
		return ""
	}
	mergedCountText := fmt.Sprintf("%s: %d merged", mergeStatsWindowText(stats), stats.MergedCount)
	medians := shownMedians(*stats, weekBefore)
	if len(medians) == 0 {
		return mergedCountText
	}
	medianTexts := utilities.Map(medians, func(median shownMedian) string { return median.text })
	lineText := mergedCountText + " · median " + strings.Join(medianTexts, ", ")
	if slices.ContainsFunc(medians, hasNoPercentChange) {
		return lineText
	}
	percentChangeTexts := utilities.Map(medians, func(median shownMedian) string {
		return percentChangeText(*median.percentChange)
	})
	return lineText + " (" + strings.Join(percentChangeTexts, ", ") + ")"
}

func shownMedians(stats mergestats.MergeStats, weekBefore *mergestats.MergeStats) []shownMedian {
	var firstReviewChange, mergeChange *int
	if weekBefore != nil {
		firstReviewChange, mergeChange = mergestats.PercentChanges(stats, *weekBefore)
	}
	var medians []shownMedian
	if stats.MedianTimeToFirstReview != nil {
		medians = append(medians, shownMedian{
			text:          mergeStatsDurationText(*stats.MedianTimeToFirstReview) + " to first review",
			percentChange: firstReviewChange,
		})
	}
	if stats.MedianTimeToMerge != nil {
		medians = append(medians, shownMedian{
			text:          mergeStatsDurationText(*stats.MedianTimeToMerge) + " to merge",
			percentChange: mergeChange,
		})
	}
	return medians
}

func hasNoPercentChange(median shownMedian) bool {
	return median.percentChange == nil
}

func percentChangeText(percentChange int) string {
	if percentChange > 0 {
		return fmt.Sprintf("+%d %%", percentChange)
	}
	return fmt.Sprintf("%d %%", percentChange)
}

// A window under 12 hours rounds to 0 and reads as 1 day. See messagecontent.spec.md § Oddities
func mergeStatsWindowText(stats *mergestats.MergeStats) string {
	windowDays := max(1, int(math.Round(stats.ComputedAt.Sub(stats.MergedSince).Hours()/24)))
	if windowDays == 1 {
		return "Last 1 day"
	}
	return fmt.Sprintf("Last %d days", windowDays)
}

// See messagecontent.spec.md § Oddities for the rounding at each switch.
func mergeStatsDurationText(duration time.Duration) string {
	switch {
	case duration < time.Hour:
		minutes := int(math.Round(duration.Minutes()))
		if minutes == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", minutes)
	case duration < 24*time.Hour:
		return fmt.Sprintf("%.1f hours", duration.Hours())
	default:
		return fmt.Sprintf("%.1f days", duration.Hours()/24)
	}
}
