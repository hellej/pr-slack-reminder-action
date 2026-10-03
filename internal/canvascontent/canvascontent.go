// Package canvascontent structures PR views into the sections a PR tracker canvas shows: the
// open PRs bucketed by next action, work-in-progress (draft) PRs and recently merged PRs.
// It carries no rendering, that belongs to canvasbuilder.
package canvascontent

import (
	"log"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/internal/prview"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

// Drafts inactive longer than this are left off the canvas.
const MaxDraftPRInactivity = 60 * 24 * time.Hour

// How many drafts without recent activity the WIP section shows.
const MaxInactiveWIPPRs = 5

type Content struct {
	ReadyToMerge         prview.PRSection
	WaitingForAuthor     prview.PRSection
	WaitingForReview     prview.PRSection
	WIP                  prview.PRSection
	Merged               prview.PRSection
	GroupedByRepository  bool
	OpenPRsCapped        bool
	WIPPRsCapped         bool
	MergedPRsUnavailable bool
	GeneratedAt          time.Time
}

type GetContentOptions struct {
	OpenPRsCapped        bool
	WIPPRsCapped         bool
	MergedPRsUnavailable bool
	GeneratedAt          time.Time
}

// GetContent splits the given PRs into the three open sections and a work-in-progress section,
// and takes the merged section from its own list. Open PRs are bucketed by next action,
// each bucket keeping the given order (oldest first); WIP PRs are ordered most recent activity
// first, with long-inactive ones dropped and the rest of the drafts without recent activity
// capped at MaxInactiveWIPPRs; merged PRs are ordered newest merge first. Each section is
// bucketed by repository when configured, in that same order.
func GetContent(
	prs []prview.PR,
	mergedPRs []prview.PR,
	contentInputs config.ContentInputs,
	options GetContentOptions,
) Content {
	sortedOpenPRs := prview.SortPRsOldestToNewest(utilities.Filter(prs, prview.PR.IsOpen))
	activeDrafts := utilities.Filter(
		utilities.Filter(prs, prview.PR.IsDraft),
		isActiveEnoughForCanvas(options.GeneratedAt),
	)
	sortedActiveDraftPRs := prview.SortPRsNewestFirst(activeDrafts, prview.PR.LastActivityAt)
	wipPRs := withInactiveDraftsCapped(sortedActiveDraftPRs, options.GeneratedAt)
	sortedMergedPRs := prview.SortPRsNewestFirst(mergedPRs, func(pr prview.PR) *time.Time {
		return pr.GetMergedAt()
	})

	readyToMerge := prsWhoseNextActionIs(sortedOpenPRs, prview.NextActionReadyToMerge)
	waitingForAuthor := prsWhoseNextActionIs(sortedOpenPRs, prview.NextActionWaitingForAuthor)
	waitingForReview := prsWhoseNextActionIs(sortedOpenPRs, prview.NextActionWaitingForReview)

	log.Printf(
		"Putting %d ready to merge, %d waiting for author and %d waiting for review pull requests, "+
			"%d work-in-progress pull requests and %d merged pull requests on the canvas, "+
			"leaving out %d inactive work-in-progress pull requests",
		len(readyToMerge), len(waitingForAuthor), len(waitingForReview),
		len(wipPRs), len(sortedMergedPRs),
		len(sortedActiveDraftPRs)-len(wipPRs),
	)

	return Content{
		ReadyToMerge:         newPRSection(readyToMerge, contentInputs),
		WaitingForAuthor:     newPRSection(waitingForAuthor, contentInputs),
		WaitingForReview:     newPRSection(waitingForReview, contentInputs),
		WIP:                  newPRSection(wipPRs, contentInputs),
		Merged:               newPRSection(sortedMergedPRs, contentInputs),
		GroupedByRepository:  contentInputs.GroupByRepository,
		OpenPRsCapped:        options.OpenPRsCapped,
		WIPPRsCapped:         options.WIPPRsCapped,
		MergedPRsUnavailable: options.MergedPRsUnavailable,
		GeneratedAt:          options.GeneratedAt,
	}
}

func prsWhoseNextActionIs(
	sortedOpenPRs []prview.PR,
	nextAction prview.PRNextAction,
) []prview.PR {
	return utilities.Filter(sortedOpenPRs, func(pr prview.PR) bool {
		return pr.GetNextAction() == nextAction
	})
}

func newPRSection(sortedPRs []prview.PR, contentInputs config.ContentInputs) prview.PRSection {
	if !contentInputs.GroupByRepository {
		return prview.PRSection{Rows: prview.RowsCollapsingAuthors(sortedPRs, contentInputs.CollapsedPRAuthors)}
	}
	return prview.PRSection{
		Groups: utilities.Map(
			prview.GroupPRsByRepositoriesInGivenOrder(sortedPRs),
			func(group prview.RepositoryPRs) prview.RepositoryRows {
				return prview.RepositoryRows{
					Repository: group.Repository,
					Rows:       prview.RowsCollapsingAuthors(group.PRs, contentInputs.CollapsedPRAuthors),
				}
			},
		),
	}
}

func isActiveEnoughForCanvas(generatedAt time.Time) func(prview.PR) bool {
	return func(pr prview.PR) bool {
		return pr.IsActiveAsOf(generatedAt, MaxDraftPRInactivity)
	}
}

func withInactiveDraftsCapped(sortedDrafts []prview.PR, generatedAt time.Time) []prview.PR {
	inactiveKept := 0
	return utilities.Filter(sortedDrafts, func(pr prview.PR) bool {
		if pr.IsActiveAsOf(generatedAt, prview.RecentActivityThreshold) {
			return true
		}
		inactiveKept++
		return inactiveKept <= MaxInactiveWIPPRs
	})
}
