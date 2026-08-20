// Package prparser enriches raw GitHub PR data with additional metadata
// for message display. It handles Slack user ID mapping, age calculation,
// and sorting of PRs for presentation.
package prparser

import (
	"maps"
	"slices"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

// A PR is marked idle once it has seen no activity for this long.
const idleThreshold = 48 * time.Hour

type PR struct {
	*githubclient.PR
	Author     Collaborator
	Approvers  []Collaborator // Users who have approved the PR at least once
	Commenters []Collaborator // Users who have commented on the PR but did not approve it
	IsOldPR    bool           // true if the PR is older than the configured threshold
}

type Collaborator struct {
	*githubclient.Collaborator
	SlackUserID string // empty string if not available
}

func NewCollaborator(c githubclient.Collaborator, slackUserId string) Collaborator {
	return Collaborator{
		Collaborator: &c,
		SlackUserID:  slackUserId,
	}
}

func (pr PR) GetPRAgeText() string {
	return durationText(time.Since(pr.GetCreatedAt()))
}

// True when the PR has seen no activity for longer than idleThreshold. A PR with unknown
// activity is not idle.
func (pr PR) IsIdle() bool {
	lastActivityAt := pr.GetLastActivityAt()
	return lastActivityAt != nil && time.Since(*lastActivityAt) > idleThreshold
}

func (pr PR) IsMerged() bool {
	return pr.GetMerged()
}

func (pr PR) IsClosedButNotMerged() bool {
	return pr.GetState() == "closed" && !pr.IsMerged()
}

func ParsePRs(prs []githubclient.PR, config config.ContentInputs) []PR {
	return sortPRsOldestToNewest(utilities.Map(prs, getPRParser(config)))
}

func getPRParser(config config.ContentInputs) func(pr githubclient.PR) PR {
	return func(pr githubclient.PR) PR {
		return parsePR(pr, config)
	}
}

func parsePR(pr githubclient.PR, config config.ContentInputs) PR {
	return PR{
		PR:         &pr,
		Author:     NewCollaborator(pr.Author, config.SlackUserIdByGitHubUsername[pr.Author.Login]),
		Approvers:  withSlackUserIds(pr.ApprovedByUsers, config.SlackUserIdByGitHubUsername),
		Commenters: withSlackUserIds(pr.CommentedByUsers, config.SlackUserIdByGitHubUsername),
		IsOldPR:    isOlderThan(pr, config.OldPRThresholdHours),
	}
}

func withSlackUserIds(
	collaborators []githubclient.Collaborator,
	slackUserIdByGitHubUsername map[string]string,
) []Collaborator {
	return utilities.Map(collaborators, func(c githubclient.Collaborator) Collaborator {
		return NewCollaborator(c, slackUserIdByGitHubUsername[c.Login])
	})
}

type RepositoryPRs struct {
	Repository models.Repository
	PRs        []PR
}

// Buckets PRs by repository, ordered alphabetically by repository path. PRs keep their given
// order within a bucket.
func GroupPRsByRepositories(prs []PR) []RepositoryPRs {
	repositoryByPath := make(map[string]models.Repository)
	prsByRepositoryPath := make(map[string][]PR)

	for _, pr := range prs {
		path := pr.Repository.GetPath()
		repositoryByPath[path] = pr.Repository
		prsByRepositoryPath[path] = append(prsByRepositoryPath[path], pr)
	}

	paths := slices.Sorted(maps.Keys(repositoryByPath))

	return utilities.Map(paths, func(path string) RepositoryPRs {
		return RepositoryPRs{
			Repository: repositoryByPath[path],
			PRs:        prsByRepositoryPath[path],
		}
	})
}

// SortPRsNewestFirst returns the PRs ordered by the given timestamp, newest first. PRs whose
// timestamp is nil are unknown rather than old, so they sort last, keeping their given order
// among themselves. The given slice is left untouched.
func SortPRsNewestFirst(prs []PR, timestamp func(PR) *time.Time) []PR {
	sorted := slices.Clone(prs)
	slices.SortStableFunc(sorted, func(a, b PR) int {
		timestampA, timestampB := timestamp(a), timestamp(b)
		if timestampA == nil && timestampB == nil {
			return 0
		}
		if timestampA == nil {
			return 1
		}
		if timestampB == nil {
			return -1
		}
		return timestampB.Compare(*timestampA)
	})
	return sorted
}

func sortPRsOldestToNewest(prs []PR) []PR {
	slices.SortStableFunc(prs, func(a, b PR) int {
		if !a.GetCreatedAt().Equal(b.GetCreatedAt()) {
			return a.GetCreatedAt().Compare(b.GetCreatedAt())
		}
		return a.GetUpdatedAt().Compare(b.GetUpdatedAt())
	})
	return prs
}

func isOlderThan(pr githubclient.PR, hours int) bool {
	if hours == 0 {
		return false
	}
	if pr.GetCreatedAt().IsZero() {
		return true
	}
	return pr.GetCreatedAt().Before(time.Now().Add(-time.Duration(hours) * time.Hour))
}
