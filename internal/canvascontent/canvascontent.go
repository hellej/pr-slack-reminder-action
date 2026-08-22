// Package canvascontent structures parsed PRs into the three sections a PR tracker canvas
// shows: open PRs, work-in-progress (draft) PRs and recently merged PRs. It carries no
// rendering, that belongs to canvasbuilder.
package canvascontent

import (
	"log"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/internal/prparser"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

// Drafts untouched for longer than this are left off the canvas.
const MaxDraftPRInactivity = 60 * 24 * time.Hour

type Content struct {
	OpenPRs                    []prparser.PR
	OpenPRsGroupedByRepository []prparser.RepositoryPRs
	GroupedByRepository        bool
	WIPPRs                     []prparser.PR
	MergedPRs                  []prparser.PR
	OpenPRsCapped              bool
	WIPPRsCapped               bool
	MergedPRsUnavailable       bool
	GeneratedAt                time.Time
}

type GetContentOptions struct {
	// Reported by the fetch, never derived from how many PRs reach the canvas.
	OpenPRsCapped bool
	WIPPRsCapped  bool
	// Set when the merged PR fetch failed, so the section can say so instead of claiming
	// that nothing was merged.
	MergedPRsUnavailable bool
	// The moment the canvas is generated: shown in the footer and used as "now" when
	// pruning inactive drafts.
	GeneratedAt time.Time
}

// GetContent splits the given PRs into an open section and a work-in-progress section, and takes
// the merged section from its own list. Open PRs keep their given order (oldest first) and are
// grouped by repository when configured; WIP PRs are always a flat list, most recent activity
// first, with long-inactive ones dropped; merged PRs are always a flat list, newest merge first.
func GetContent(
	prs []prparser.PR,
	mergedPRs []prparser.PR,
	contentInputs config.ContentInputs,
	options GetContentOptions,
) Content {
	openPRs := utilities.Filter(prs, func(pr prparser.PR) bool { return !pr.GetDraft() })
	drafts := utilities.Filter(prs, func(pr prparser.PR) bool { return pr.GetDraft() })
	activeDrafts := utilities.Filter(drafts, isActiveEnoughForCanvas(options.GeneratedAt))

	log.Printf(
		"Putting %d open pull requests, %d work-in-progress pull requests and %d merged pull requests on the canvas (%d drafts left out as inactive)",
		len(openPRs), len(activeDrafts), len(mergedPRs), len(drafts)-len(activeDrafts),
	)

	content := Content{
		WIPPRs: prparser.SortPRsNewestFirst(activeDrafts, func(pr prparser.PR) *time.Time {
			return pr.GetLastActivityAt()
		}),
		// ParsePRs re-sorts every PR oldest first, so the merge order is made here.
		MergedPRs: prparser.SortPRsNewestFirst(mergedPRs, func(pr prparser.PR) *time.Time {
			return pr.GetMergedAt()
		}),
		GroupedByRepository:  contentInputs.GroupByRepository,
		OpenPRsCapped:        options.OpenPRsCapped,
		WIPPRsCapped:         options.WIPPRsCapped,
		MergedPRsUnavailable: options.MergedPRsUnavailable,
		GeneratedAt:          options.GeneratedAt,
	}

	if contentInputs.GroupByRepository {
		content.OpenPRsGroupedByRepository = prparser.GroupPRsByRepositories(openPRs)
		return content
	}
	content.OpenPRs = openPRs
	return content
}

// Unknown activity is not staleness, so a draft without a last activity time is kept.
func isActiveEnoughForCanvas(generatedAt time.Time) func(prparser.PR) bool {
	inactiveBefore := generatedAt.Add(-MaxDraftPRInactivity)
	return func(pr prparser.PR) bool {
		lastActivityAt := pr.GetLastActivityAt()
		return lastActivityAt == nil || !lastActivityAt.Before(inactiveBefore)
	}
}
