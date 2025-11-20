// Package prparser enriches raw GitHub PR data with additional metadata
// for message display. It handles Slack user ID mapping, age calculation,
// prefixes (for PR link texts), and sorting of PRs for presentation.
package prparser

import (
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

type PR struct {
	*githubclient.PR
	Author     Collaborator
	Approvers  []Collaborator // Users who have approved the PR at least once
	Commenters []Collaborator // Users who have commented on the PR but did not approve it
	IsOldPR    bool           // true if the PR is older than the configured threshold
	Prefix     string
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
	duration := time.Since(pr.CreatedAt.Time)
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

func (pr PR) IsMerged() bool {
	return pr.GetMerged()
}

func (pr PR) IsClosedButNotMerged() bool {
	return pr.GetState() == "closed" && !pr.IsMerged()
}

func ParsePRs(prs []githubclient.PR, config config.ContentInputs) []PR {
	return sortPRsByCreatedAt(utilities.Map(prs, getPRParser(config)))
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
		Prefix:     config.GetPRLinkRepoPrefix(pr.Repository),
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

func sortPRsByCreatedAt(prs []PR) []PR {
	slices.SortStableFunc(prs, func(a, b PR) int {
		if !a.GetCreatedAt().Time.Equal(b.GetCreatedAt().Time) {
			return b.GetCreatedAt().Time.Compare(a.GetCreatedAt().Time)
		}
		return b.GetUpdatedAt().Time.Compare(a.GetUpdatedAt().Time)
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
