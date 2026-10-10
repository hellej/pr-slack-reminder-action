// Package messagecontent structures PR views into the sections a reminder message shows: the
// open PRs bucketed by next action, and the PRs that recently merged. It carries no rendering,
// that belongs to messagebuilder.
package messagecontent

import (
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/internal/prview"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

const MaxUntrackedPRsMergedBeforePost = 3

const noOpenPRsSummaryText = "Nothing waiting for review 🎉"

type Content struct {
	SummaryText         string
	ReadyToMerge        prview.PRSection
	WaitingForAuthor    prview.PRSection
	WaitingForReview    prview.PRSection
	Merged              prview.PRSection
	NoOpenPRsText       string
	GeneratedAt         time.Time
	GroupedByRepository bool
}

func (c Content) HasPRs() bool {
	return c.ReadyToMerge.HasRows() || c.WaitingForAuthor.HasRows() ||
		c.WaitingForReview.HasRows() || c.Merged.HasRows()
}

// See messagecontent.spec.md for this function's full behaviour.
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
		"Putting %d waiting for review, %d waiting for author and %d ready to merge pull requests "+
			"and %d merged pull requests in the message",
		len(waitingForReview), len(waitingForAuthor), len(readyToMerge), len(mergedPRs),
	)

	content := Content{
		SummaryText:         getSummaryText(len(waitingForReview), len(waitingForAuthor), len(readyToMerge)),
		ReadyToMerge:        newPRSection(readyToMerge, contentInputs),
		WaitingForAuthor:    newPRSection(waitingForAuthor, contentInputs),
		WaitingForReview:    newPRSection(waitingForReview, contentInputs),
		Merged:              newPRSection(mergedPRs, contentInputs),
		GeneratedAt:         generatedAt,
		GroupedByRepository: contentInputs.GroupByRepository,
	}
	if len(sortedOpenPRs) == 0 {
		content.NoOpenPRsText = contentInputs.NoPRsMessage
	}
	return content
}

// Sorting the fetch before capping keeps the cap from resting on another package's ordering.
func selectMergedPRsToShow(
	trackedPRs []prview.PR,
	recentlyMergedPRs []prview.PR,
	messagePostedAt time.Time,
) []prview.PR {
	trackedMergedPRs := utilities.Filter(trackedPRs, prview.PR.IsMerged)
	trackedMergedPRRefs := utilities.Map(trackedMergedPRs, prview.PR.GetPullRequestRef)
	untrackedMergedPRs := utilities.Filter(
		sortByMergeTimeNewestFirst(recentlyMergedPRs),
		func(pr prview.PR) bool { return !slices.Contains(trackedMergedPRRefs, pr.GetPullRequestRef()) },
	)
	isMergedSincePost := func(pr prview.PR) bool {
		return !messagePostedAt.IsZero() && pr.GetMergedAt() != nil && pr.GetMergedAt().After(messagePostedAt)
	}
	mergedSincePost := utilities.Filter(untrackedMergedPRs, isMergedSincePost)
	mergedBeforePost := utilities.Filter(untrackedMergedPRs, func(pr prview.PR) bool {
		return !isMergedSincePost(pr)
	})
	if len(mergedBeforePost) > MaxUntrackedPRsMergedBeforePost {
		mergedBeforePost = mergedBeforePost[:MaxUntrackedPRsMergedBeforePost]
	}
	return sortByMergeTimeNewestFirst(slices.Concat(trackedMergedPRs, mergedSincePost, mergedBeforePost))
}

func sortByMergeTimeNewestFirst(prs []prview.PR) []prview.PR {
	return prview.SortPRsNewestFirst(prs, func(pr prview.PR) *time.Time { return pr.GetMergedAt() })
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
		return prview.PRSection{Rows: prview.RowsCollapsingPRsFromAuthors(sortedPRs, contentInputs.CollapsePRsFromAuthors)}
	}
	return prview.PRSection{
		Groups: utilities.Map(
			prview.GroupPRsByRepositoriesInGivenOrder(sortedPRs),
			func(group prview.RepositoryPRs) prview.RepositoryRows {
				return prview.RepositoryRows{
					Repository: group.Repository,
					Rows:       prview.RowsCollapsingPRsFromAuthors(group.PRs, contentInputs.CollapsePRsFromAuthors),
				}
			},
		),
	}
}

type summaryPart struct {
	prCount    int
	nextAction string
}

func getSummaryText(waitingForReviewCount, waitingForAuthorCount, readyToMergeCount int) string {
	nonEmptyParts := utilities.Filter([]summaryPart{
		{prCount: waitingForReviewCount, nextAction: "to review"},
		{prCount: waitingForAuthorCount, nextAction: "waiting for author"},
		{prCount: readyToMergeCount, nextAction: "to merge"},
	}, func(part summaryPart) bool { return part.prCount > 0 })
	if len(nonEmptyParts) == 0 {
		return noOpenPRsSummaryText
	}
	first := nonEmptyParts[0]
	firstPartText := fmt.Sprintf("%d %s %s", first.prCount, prNoun(first.prCount), first.nextAction)
	laterPartTexts := utilities.Map(nonEmptyParts[1:], func(part summaryPart) string {
		return fmt.Sprintf("%d %s", part.prCount, part.nextAction)
	})
	return strings.Join(slices.Concat([]string{firstPartText}, laterPartTexts), ", ") + " 👀"
}

func prNoun(prCount int) string {
	if prCount == 1 {
		return "PR"
	}
	return "PRs"
}
