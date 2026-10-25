package messagecontent

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/mergestats"
)

func mergeStatsText(stats *mergestats.MergeStats) string {
	if stats == nil {
		return ""
	}
	mergedCountText := fmt.Sprintf("%s: %d merged", mergeStatsWindowText(stats), stats.MergedCount)
	var medianTexts []string
	if stats.MedianTimeToFirstReview != nil {
		medianTexts = append(medianTexts, mergeStatsDurationText(*stats.MedianTimeToFirstReview)+" to first review")
	}
	if stats.MedianTimeToMerge != nil {
		medianTexts = append(medianTexts, mergeStatsDurationText(*stats.MedianTimeToMerge)+" to merge")
	}
	if len(medianTexts) == 0 {
		return mergedCountText
	}
	return mergedCountText + " · median " + strings.Join(medianTexts, ", ")
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
