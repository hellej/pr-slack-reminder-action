// Package messagecontent structures PR views into the sections a reminder message shows: the
// open PRs bucketed by next action, and the PRs that recently merged. It carries no rendering,
// that belongs to messagebuilder.
package messagecontent

import (
	"fmt"
	"log"
	"slices"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/prview"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

// How many merged PRs the message picks off the recently-merged fetch from before the message
// was posted. The merged PRs the message already tracks, and those merged since the post, are
// never counted against it.
const MaxUntrackedMergedPRs = 3

const noOpenPRsSummaryText = "Nothing waiting for review 🎉"

// One message section's PRs, either as the flat list or as repository buckets.
type PRSection struct {
	PRs    []prview.PR
	Groups []PRsOfRepository
}

func (section PRSection) HasPRs() bool {
	return len(section.PRs) > 0 || len(section.Groups) > 0
}

type PRsOfRepository struct {
	RepositoryName string
	PRs            []prview.PR
}

type Content struct {
	SummaryText      string
	ReadyToMerge     PRSection
	WaitingForAuthor PRSection
	WaitingForReview PRSection
	Merged           PRSection
	// The configured no-PRs message, set only when no open PR is listed.
	NoOpenPRsText       string
	GeneratedAt         time.Time
	GroupedByRepository bool
}

func (c Content) HasPRs() bool {
	return c.ReadyToMerge.HasPRs() || c.WaitingForAuthor.HasPRs() ||
		c.WaitingForReview.HasPRs() || c.Merged.HasPRs()
}

// GetContent splits the open PRs into the three next-action sections, oldest first, and takes
// the merged section from the tracked PRs that have since merged, every PR merged since the post,
// and the newest untracked merges from before it. Each section is bucketed by repository when
// configured, in that same order.
//
// openPRs are open right now and already draft-filtered by the caller. trackedPRs are the PRs
// the message was posted with, as re-fetched, in whatever state they are now: only the merged
// ones are read. A zero messagePostedAt means no message is posted yet.
func GetContent(
	openPRs []prview.PR,
	trackedPRs []prview.PR,
	recentlyMergedPRs []prview.PR,
	messagePostedAt time.Time,
	generatedAt time.Time,
	contentInputs config.ContentInputs,
) Content {
	sortedOpenPRs := prview.SortPRsOldestToNewest(openPRs)
	readyToMerge := prsWhoseNextActionIs(sortedOpenPRs, prview.NextActionReadyToMerge)
	waitingForAuthor := prsWhoseNextActionIs(sortedOpenPRs, prview.NextActionWaitingForAuthor)
	waitingForReview := prsWhoseNextActionIs(sortedOpenPRs, prview.NextActionWaitingForReview)
	mergedPRs := selectMergedPRsToShow(trackedPRs, recentlyMergedPRs, messagePostedAt)

	log.Printf(
		"Putting %d ready to merge, %d waiting for author and %d waiting for review pull requests "+
			"and %d merged pull requests in the message",
		len(readyToMerge), len(waitingForAuthor), len(waitingForReview), len(mergedPRs),
	)

	groupByRepository := contentInputs.GroupByRepository
	content := Content{
		SummaryText:         getSummaryText(len(sortedOpenPRs)),
		ReadyToMerge:        newPRSection(readyToMerge, groupByRepository),
		WaitingForAuthor:    newPRSection(waitingForAuthor, groupByRepository),
		WaitingForReview:    newPRSection(waitingForReview, groupByRepository),
		Merged:              newPRSection(mergedPRs, groupByRepository),
		GeneratedAt:         generatedAt,
		GroupedByRepository: groupByRepository,
	}
	if len(sortedOpenPRs) == 0 {
		content.NoOpenPRsText = contentInputs.NoPRsMessage
	}
	return content
}

// The tracked merges and those since the post are kept whole, and the cap applies to the
// fetch's merges from before the post alone. Sorting the fetch before capping keeps the 3 newest
// of it from resting on another package's ordering.
func selectMergedPRsToShow(
	trackedPRs []prview.PR,
	recentlyMergedPRs []prview.PR,
	messagePostedAt time.Time,
) []prview.PR {
	trackedMergedPRs := utilities.Filter(trackedPRs, prview.PR.IsMerged)
	isTrackedByPRRef := getIsTrackedByPRRefMap(trackedMergedPRs)
	untrackedMergedPRs := utilities.Filter(
		sortByMergeTimeNewestFirst(recentlyMergedPRs),
		func(pr prview.PR) bool { return !isTrackedByPRRef[pr.GetPullRequestRef()] },
	)
	isMergedSincePost := func(pr prview.PR) bool {
		return !messagePostedAt.IsZero() && pr.GetMergedAt() != nil && pr.GetMergedAt().After(messagePostedAt)
	}
	mergedSincePost := utilities.Filter(untrackedMergedPRs, isMergedSincePost)
	mergedBeforePost := utilities.Filter(untrackedMergedPRs, func(pr prview.PR) bool {
		return !isMergedSincePost(pr)
	})
	if len(mergedBeforePost) > MaxUntrackedMergedPRs {
		mergedBeforePost = mergedBeforePost[:MaxUntrackedMergedPRs]
	}
	return sortByMergeTimeNewestFirst(slices.Concat(trackedMergedPRs, mergedSincePost, mergedBeforePost))
}

func sortByMergeTimeNewestFirst(prs []prview.PR) []prview.PR {
	return prview.SortPRsNewestFirst(prs, func(pr prview.PR) *time.Time { return pr.GetMergedAt() })
}

func getIsTrackedByPRRefMap(trackedPRs []prview.PR) map[models.PullRequestRef]bool {
	isTrackedByPRRef := make(map[models.PullRequestRef]bool, len(trackedPRs))
	for _, pr := range trackedPRs {
		isTrackedByPRRef[pr.GetPullRequestRef()] = true
	}
	return isTrackedByPRRef
}

func prsWhoseNextActionIs(
	sortedOpenPRs []prview.PR,
	nextAction prview.PRNextAction,
) []prview.PR {
	return utilities.Filter(sortedOpenPRs, func(pr prview.PR) bool {
		return pr.GetNextAction() == nextAction
	})
}

func newPRSection(sortedPRs []prview.PR, groupByRepository bool) PRSection {
	if !groupByRepository {
		return PRSection{PRs: sortedPRs}
	}
	return PRSection{
		Groups: utilities.Map(
			prview.GroupPRsByRepositoriesInGivenOrder(sortedPRs),
			func(group prview.RepositoryPRs) PRsOfRepository {
				return PRsOfRepository{
					RepositoryName: group.Repository.Name,
					PRs:            group.PRs,
				}
			},
		),
	}
}

func getSummaryText(openPRCount int) string {
	switch openPRCount {
	case 0:
		return noOpenPRsSummaryText
	case 1:
		return "1 open PR is waiting for attention 👀"
	default:
		return fmt.Sprintf("%d open PRs are waiting for attention 👀", openPRCount)
	}
}
