package prparser

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
