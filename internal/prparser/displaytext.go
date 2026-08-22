package prparser

import (
	"fmt"
	"math"
	"time"
)

// Renders a duration as days, hours or minutes depending on its magnitude, always plural and
// rounded to whole units.
func durationText(duration time.Duration) string {
	if duration.Hours() >= 24 {
		days := int(math.Round(duration.Hours())) / 24
		return fmt.Sprintf("%d days", days)
	} else if duration.Hours() >= 1 {
		hours := int(math.Round(duration.Hours()))
		return fmt.Sprintf("%d hours", hours)
	} else {
		minutes := int(math.Round(duration.Minutes()))
		return fmt.Sprintf("%d minutes", minutes)
	}
}

// GetActivityText renders how long ago the PR last saw activity: "updated N minutes/hours ago"
// under a day, "idle N days" from a day onwards. Unknown activity yields no text.
func (pr PR) GetActivityText() string {
	lastActivityAt := pr.GetLastActivityAt()
	if lastActivityAt == nil {
		return ""
	}
	inactivity := time.Since(*lastActivityAt)
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
