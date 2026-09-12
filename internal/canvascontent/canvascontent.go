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
	// The open PRs, bucketed by whose turn it is. A reader picks their next action off the
	// heading, so the buckets replace one flat open section.
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
	sortedOpenPRs := prparser.SortPRsOldestToNewest(utilities.Filter(prs, isOpen))
	activeDrafts := utilities.Filter(
		utilities.Filter(prs, isDraft),
		isActiveEnoughForCanvas(options.GeneratedAt),
	)
	sortedActiveDraftPRs := prparser.SortPRsNewestFirst(activeDrafts, lastActivityAt)
	wipPRs := withInactiveDraftsCapped(sortedActiveDraftPRs, options.GeneratedAt)
	sortedMergedPRs := prparser.SortPRsNewestFirst(mergedPRs, func(pr prparser.PR) *time.Time {
		return pr.GetMergedAt()
	})

	readyToMerge := prsWhoseTurnIs(sortedOpenPRs, prparser.TurnReadyToMerge)
	waitingForAuthor := prsWhoseTurnIs(sortedOpenPRs, prparser.TurnWaitingForAuthor)
	waitingForReview := prsWhoseTurnIs(sortedOpenPRs, prparser.TurnWaitingForReview)

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
		ReadyToMerge:         prSection(readyToMerge, groupByRepository),
		WaitingForAuthor:     prSection(waitingForAuthor, groupByRepository),
		WaitingForReview:     prSection(waitingForReview, groupByRepository),
		WIP:                  prSection(wipPRs, groupByRepository),
		Merged:               prSection(sortedMergedPRs, groupByRepository),
		GroupedByRepository:  groupByRepository,
		OpenPRsCapped:        options.OpenPRsCapped,
		WIPPRsCapped:         options.WIPPRsCapped,
		MergedPRsUnavailable: options.MergedPRsUnavailable,
		GeneratedAt:          options.GeneratedAt,
	}
}

// Filtering the sorted list rather than sorting each bucket is what keeps every bucket oldest
// first.
func prsWhoseTurnIs(sortedOpenPRs []prparser.PR, turn prparser.PRTurn) []prparser.PR {
	return utilities.Filter(sortedOpenPRs, func(pr prparser.PR) bool {
		return prparser.GetPRTurn(pr) == turn
	})
}

// Fills the one shape the canvas will read, never both. The given list is already in its
// section's order, so bucketing it in that order puts the repository holding the section's
// leading PR first.
func prSection(sortedPRs []prparser.PR, groupByRepository bool) PRSection {
	if groupByRepository {
		return PRSection{Groups: prparser.GroupPRsByRepositoriesInGivenOrder(sortedPRs)}
	}
	return PRSection{PRs: sortedPRs}
}

// Keeps every recently active draft and the MaxInactiveWIPPRs most recently active inactive
// ones. The given list is sorted most recent activity first, so the drop hits the least
// recently touched drafts.
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

// Unknown activity is not staleness, so a draft without an update time is kept.
func isActiveEnoughForCanvas(generatedAt time.Time) func(prparser.PR) bool {
	inactiveBefore := generatedAt.Add(-MaxDraftPRInactivity)
	return func(pr prparser.PR) bool {
		updatedAt := pr.GetUpdatedAt()
		return updatedAt.IsZero() || !updatedAt.Before(inactiveBefore)
	}
}

// Names the update time as the activity the WIP section sorts on, and spells the unknown case
// as the nil SortPRsNewestFirst documents. A zero time would sort last on its own, being year 1.
func lastActivityAt(pr prparser.PR) *time.Time {
	updatedAt := pr.GetUpdatedAt()
	if updatedAt.IsZero() {
		return nil
	}
	return &updatedAt
}

func isOpen(pr prparser.PR) bool { return !pr.GetDraft() }

func isDraft(pr prparser.PR) bool { return pr.GetDraft() }
