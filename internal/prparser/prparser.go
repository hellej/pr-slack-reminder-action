// Package prparser enriches raw GitHub PR data with additional metadata
// for message and canvas display. It handles Slack user ID mapping, age and
// activity calculation, sorting, and grouping by repository. It also renders
// the reviewer, activity and merged-time texts a PR row shows.
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

// A PR counts as recently updated until its last activity is this old. GetActivityText flips
// its wording at the same boundary.
const recentActivityThreshold = 24 * time.Hour

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

// True when the PR saw activity less than recentActivityThreshold ago. A PR with unknown
// activity, a zero update time, is not recently updated.
func (pr PR) IsRecentlyUpdated() bool {
	updatedAt := pr.GetUpdatedAt()
	return !updatedAt.IsZero() && time.Since(updatedAt) < recentActivityThreshold
}

func (pr PR) IsMerged() bool {
	return pr.GetMerged()
}

func (pr PR) IsClosedButNotMerged() bool {
	return pr.GetState() == "closed" && !pr.IsMerged()
}

func ParsePRs(prs []githubclient.PR, config config.ContentInputs) []PR {
	return utilities.Map(prs, getPRParser(config))
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
	buckets := bucketPRsByRepository(prs)
	return buckets.groupsForPaths(slices.Sorted(maps.Keys(buckets.repositoryByPath)))
}

// Buckets PRs by repository, ordered by each repository's first PR in the given list. PRs keep
// their given order within a bucket, so an already-sorted list decides both orders.
func GroupPRsByRepositoriesInGivenOrder(prs []PR) []RepositoryPRs {
	buckets := bucketPRsByRepository(prs)
	pathsInGivenOrder := utilities.UniqueFunc(
		utilities.Map(prs, func(pr PR) string { return pr.Repository.GetPath() }),
		func(a, b string) bool { return a == b },
	)
	return buckets.groupsForPaths(pathsInGivenOrder)
}

type repositoryBuckets struct {
	repositoryByPath    map[string]models.Repository
	prsByRepositoryPath map[string][]PR
}

func bucketPRsByRepository(prs []PR) repositoryBuckets {
	buckets := repositoryBuckets{
		repositoryByPath:    make(map[string]models.Repository),
		prsByRepositoryPath: make(map[string][]PR),
	}
	for _, pr := range prs {
		path := pr.Repository.GetPath()
		buckets.repositoryByPath[path] = pr.Repository
		buckets.prsByRepositoryPath[path] = append(buckets.prsByRepositoryPath[path], pr)
	}
	return buckets
}

func (buckets repositoryBuckets) groupsForPaths(paths []string) []RepositoryPRs {
	return utilities.Map(paths, func(path string) RepositoryPRs {
		return RepositoryPRs{
			Repository: buckets.repositoryByPath[path],
			PRs:        buckets.prsByRepositoryPath[path],
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

func SortPRsOldestToNewest(prs []PR) []PR {
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
