// Package canvascontent structures parsed PRs into the sections a PR tracker canvas shows: the
// open PRs bucketed by whose turn it is, work-in-progress (draft) PRs and recently merged PRs.
// It carries no rendering, that belongs to canvasbuilder.
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

// How many drafts without recent activity the WIP section shows. Recently touched drafts are
// the point of the section, older ones only need a sample.
const MaxInactiveWIPPRs = 5

// One canvas section's PRs, either as the flat list or as repository buckets. Filling both
// loses one of them: canvasbuilder renders the shape Content.GroupedByRepository names.
type PRSection struct {
	PRs    []prparser.PR
	Groups []prparser.RepositoryPRs
}

type Content struct {
	ReadyToMerge         PRSection
	WaitingForAuthor     PRSection
	WaitingForReview     PRSection
	WIP                  PRSection
	Merged               PRSection
	GroupedByRepository  bool
	OpenPRsCapped        bool
	WIPPRsCapped         bool
	MergedPRsUnavailable bool
	GeneratedAt          time.Time
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

// GetContent splits the given PRs into the three open sections and a work-in-progress section,
// and takes the merged section from its own list. Open PRs are bucketed by whose turn it is,
// each bucket keeping the given order (oldest first); WIP PRs are ordered most recent activity
// first, with long-inactive ones dropped and the rest of the drafts without recent activity
// capped at MaxInactiveWIPPRs; merged PRs are ordered newest merge first. Each section is
// bucketed by repository when configured, in that same order.
func GetContent(
	prs []prparser.PR,
	mergedPRs []prparser.PR,
	contentInputs config.ContentInputs,
	options GetContentOptions,
) Content {
	sortedOpenPRs := prparser.SortPRsOldestToNewest(utilities.Filter(prs, prparser.PR.IsOpen))
	activeDrafts := utilities.Filter(
		utilities.Filter(prs, prparser.PR.IsDraft),
		isActiveEnoughForCanvas(options.GeneratedAt),
	)
	sortedActiveDraftPRs := prparser.SortPRsNewestFirst(activeDrafts, prparser.PR.LastActivityAt)
	wipPRs := withInactiveDraftsCapped(sortedActiveDraftPRs, options.GeneratedAt)
	sortedMergedPRs := prparser.SortPRsNewestFirst(mergedPRs, func(pr prparser.PR) *time.Time {
		return pr.GetMergedAt()
	})

	readyToMerge := includePRsWhoseTurnIs(sortedOpenPRs, prparser.TurnReadyToMerge)
	waitingForAuthor := includePRsWhoseTurnIs(sortedOpenPRs, prparser.TurnWaitingForAuthor)
	waitingForReview := includePRsWhoseTurnIs(sortedOpenPRs, prparser.TurnWaitingForReview)

	log.Printf(
		"Putting %d ready to merge, %d waiting for author and %d waiting for review pull requests, "+
			"%d work-in-progress pull requests and %d merged pull requests on the canvas, "+
			"leaving out %d inactive work-in-progress pull requests",
		len(readyToMerge), len(waitingForAuthor), len(waitingForReview),
		len(wipPRs), len(sortedMergedPRs),
		len(sortedActiveDraftPRs)-len(wipPRs),
	)

	groupByRepository := contentInputs.GroupByRepository
	return Content{
		ReadyToMerge:         newPRSection(readyToMerge, groupByRepository),
		WaitingForAuthor:     newPRSection(waitingForAuthor, groupByRepository),
		WaitingForReview:     newPRSection(waitingForReview, groupByRepository),
		WIP:                  newPRSection(wipPRs, groupByRepository),
		Merged:               newPRSection(sortedMergedPRs, groupByRepository),
		GroupedByRepository:  groupByRepository,
		OpenPRsCapped:        options.OpenPRsCapped,
		WIPPRsCapped:         options.WIPPRsCapped,
		MergedPRsUnavailable: options.MergedPRsUnavailable,
		GeneratedAt:          options.GeneratedAt,
	}
}

func includePRsWhoseTurnIs(sortedOpenPRs []prparser.PR, turn prparser.PRTurn) []prparser.PR {
	return utilities.Filter(sortedOpenPRs, func(pr prparser.PR) bool {
		return pr.GetTurn() == turn
	})
}

func newPRSection(sortedPRs []prparser.PR, groupByRepository bool) PRSection {
	if groupByRepository {
		return PRSection{Groups: prparser.GroupPRsByRepositoriesInGivenOrder(sortedPRs)}
	}
	return PRSection{PRs: sortedPRs}
}

func withInactiveDraftsCapped(sortedDrafts []prparser.PR, generatedAt time.Time) []prparser.PR {
	inactiveKept := 0
	return utilities.Filter(sortedDrafts, func(pr prparser.PR) bool {
		if !isInactive(pr, generatedAt) {
			return true
		}
		inactiveKept++
		return inactiveKept <= MaxInactiveWIPPRs
	})
}

// Inactive from prparser.RecentActivityThreshold of silence onwards, the boundary the WIP row
// styling uses too. Unknown activity is not inactivity, so a draft without an update time is
// never capped away.
func isInactive(pr prparser.PR, generatedAt time.Time) bool {
	updatedAt := pr.GetUpdatedAt()
	return !updatedAt.IsZero() && !updatedAt.After(generatedAt.Add(-prparser.RecentActivityThreshold))
}

// A draft with unknown update time is kept
func isActiveEnoughForCanvas(generatedAt time.Time) func(prparser.PR) bool {
	inactiveBefore := generatedAt.Add(-MaxDraftPRInactivity)
	return func(pr prparser.PR) bool {
		updatedAt := pr.GetUpdatedAt()
		return updatedAt.IsZero() || !updatedAt.Before(inactiveBefore)
	}
}
