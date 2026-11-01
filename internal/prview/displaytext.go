package prview

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

// Renders a duration as days, hours or minutes depending on its magnitude, rounded to whole
// units. The unit is singular for a count of 1, plural otherwise (0 included).
func durationText(duration time.Duration) string {
	if duration.Hours() >= 24 {
		days := int(math.Round(duration.Hours())) / 24
		return fmt.Sprintf("%d %s", days, pluralize(days, "day"))
	} else if duration.Hours() >= 1 {
		hours := int(math.Round(duration.Hours()))
		return fmt.Sprintf("%d %s", hours, pluralize(hours, "hour"))
	} else {
		minutes := int(math.Round(duration.Minutes()))
		return fmt.Sprintf("%d %s", minutes, pluralize(minutes, "minute"))
	}
}

func pluralize(count int, unit string) string {
	if count == 1 {
		return unit
	}
	return unit + "s"
}

// GetActivityText renders how long ago the PR last saw activity: "updated N minutes/hours ago"
// under a day, "idle N days" from a day onwards. Unknown activity, a zero update time, yields
// no text.
func (pr PR) GetActivityText() string {
	updatedAt := pr.GetUpdatedAt()
	if updatedAt.IsZero() {
		return ""
	}
	inactivity := time.Since(updatedAt)
	if inactivity.Hours() >= 24 {
		return "idle " + durationText(inactivity)
	}
	return "updated " + durationText(inactivity) + " ago"
}

// GetMergedText renders how long ago the PR was merged as "merged N minutes/hours/days ago".
// The prefix keeps it apart from the age an open row shows in the same style. A PR that was
// never merged yields no text.
func (pr PR) GetMergedText() string {
	mergedAt := pr.GetMergedAt()
	if mergedAt == nil {
		return ""
	}
	return "merged " + durationText(time.Since(*mergedAt)) + " ago"
}

// GetPRAgeDisplayText renders the PR's age as "N days old" past the old-PR threshold and
// "N days ago" otherwise. The warning marker that goes with an old PR belongs to the renderer.
func (pr PR) GetPRAgeDisplayText() string {
	if pr.IsOldPR {
		return pr.GetPRAgeText() + " old"
	}
	return pr.GetPRAgeText() + " ago"
}

// GetReviewersTextSegments renders the reviewer groups as "(✅ a, b / 🔁 c / 💬 d)", split into
// one text run per segment so a renderer can style or escape the names separately from the glue.
// A group without reviewers is left out. Merge blockers add a last glue segment, " / " and the
// blockers joined by ", ". No reviewers and no blockers yield no segments.
func GetReviewersTextSegments(
	approvers, staleApprovers, commenters []Collaborator, mergeBlockers []MergeBlocker,
) []string {
	groups := utilities.Filter([]reviewerGroup{
		{marker: "✅", reviewers: approvers},
		{marker: "🔁", reviewers: staleApprovers},
		{marker: "💬", reviewers: commenters},
	}, reviewerGroup.hasReviewers)

	if len(groups) == 0 {
		if len(mergeBlockers) > 0 {
			return []string{" (" + mergeBlockersText(mergeBlockers) + ")"}
		}
		return nil
	}

	var segments []string
	for index, group := range groups {
		segments = append(segments, textBeforeGroup(index)+group.marker+" ")
		segments = append(segments, nameSegments(group.reviewers)...)
	}
	if len(mergeBlockers) > 0 {
		segments = append(segments, " / "+mergeBlockersText(mergeBlockers))
	}
	return append(segments, ")")
}

type reviewerGroup struct {
	marker    string
	reviewers []Collaborator
}

func (group reviewerGroup) hasReviewers() bool {
	return len(group.reviewers) > 0
}

func textBeforeGroup(groupIndex int) string {
	if groupIndex == 0 {
		return " ("
	}
	return " / "
}

func mergeBlockersText(mergeBlockers []MergeBlocker) string {
	return strings.Join(utilities.Map(mergeBlockers, func(blocker MergeBlocker) string { return string(blocker) }), ", ")
}

func nameSegments(collaborators []Collaborator) []string {
	names := utilities.Map(collaborators, Collaborator.GetGitHubName)
	return utilities.Intersperse(names, ", ")
}
