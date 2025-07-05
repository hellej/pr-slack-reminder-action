package prparser

import (
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
)

type PR struct {
	*githubclient.PR
	Author     Collaborator
	Approvers  []Collaborator // Users who have approved the PR at least once
	Commenters []Collaborator // Users who have commented on the PR but did not approve it
}

type Collaborator struct {
	*githubclient.Collaborator
	SlackUserID string // empty string if not available
}

func NewCollaborator(c *githubclient.Collaborator, slackUserId string) Collaborator {
	return Collaborator{
		Collaborator: c,
		SlackUserID:  slackUserId,
	}
}

func (pr PR) GetPRAgeText() string {
	duration := time.Since(pr.CreatedAt.Time)
	if duration.Hours() >= 24 {
		days := int(math.Round(duration.Hours())) / 24
		return fmt.Sprintf("%d days ago", days)
	} else if duration.Hours() >= 1 {
		hours := int(math.Round(duration.Hours()))
		return fmt.Sprintf("%d hours ago", hours)
	} else {
		minutes := int(math.Round(duration.Minutes()))
		return fmt.Sprintf("%d minutes ago", minutes)
	}
}

func ParsePRs(prs []githubclient.PR, slackUserIdByGitHubUsername map[string]string) []PR {
	var parsedPRs []PR
	for _, pr := range prs {
		parsedPRs = append(parsedPRs, parsePR(pr, slackUserIdByGitHubUsername))
	}
	return sortPRsByCreatedAt(parsedPRs)
}

func parsePR(pr githubclient.PR, slackUserIdByGitHubUsername map[string]string) PR {
	return PR{
		PR:         &pr,
		Author:     NewCollaborator(&pr.Author, slackUserIdByGitHubUsername[pr.Author.Login]),
		Approvers:  withSlackUserIds(pr.ApprovedByUsers, slackUserIdByGitHubUsername),
		Commenters: withSlackUserIds(pr.CommentedByUsers, slackUserIdByGitHubUsername),
	}
}

func withSlackUserIds(
	collaborators []githubclient.Collaborator,
	slackUserIdByGitHubUsername map[string]string,
) []Collaborator {
	result := make([]Collaborator, len(collaborators))
	for i, c := range collaborators {
		result[i] = NewCollaborator(&c, slackUserIdByGitHubUsername[c.Login])
	}
	return result
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
