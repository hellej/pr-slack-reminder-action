package prparser

import (
	"fmt"
	"math"
	"time"
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

// GetReviewersTextSegments renders approvers and commenters as "(✅ a, b / 💬 c)", split into one
// text run per segment so a renderer can style or escape the names separately from the glue.
// No reviewers at all yields no segments. Approvers and commenters are given explicitly, so a
// caller wanting the commenters-only rendering passes no approvers.
func GetReviewersTextSegments(approvers, commenters []Collaborator) []string {
	if len(approvers) == 0 && len(commenters) == 0 {
		return nil
	}

	segments := []string{" (💬 "}
	if len(approvers) > 0 {
		segments = []string{" (✅ "}
		segments = append(segments, nameSegments(approvers)...)
		if len(commenters) > 0 {
			segments = append(segments, " / 💬 ")
		}
	}
	segments = append(segments, nameSegments(commenters)...)

	return append(segments, ")")
}

// Names separated by ", " segments.
func nameSegments(collaborators []Collaborator) []string {
	var segments []string
	for idx, collaborator := range collaborators {
		if idx > 0 {
			segments = append(segments, ", ")
		}
		segments = append(segments, collaborator.GetGitHubName())
	}
	return segments
}
