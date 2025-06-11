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
	AuthorInfo Collaborator
	Approvers  []Collaborator // Users who have approved the PR at least once
	Commenters []Collaborator // Users who have commented on the PR but did not approve it
}

type Collaborator struct {
	GitHubName  string // GitHub name if available, otherwise username
	SlackUserID string // empty string if not available
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
		parsedPRs = append(parsedPRs, parsePR(pr, &slackUserIdByGitHubUsername))
	}
	return sortPRsByCreatedAt(parsedPRs)
}

func parsePR(pr githubclient.PR, slackUserIdByGitHubUsername *map[string]string) PR {
	if slackUserIdByGitHubUsername == nil || len(*slackUserIdByGitHubUsername) == 0 {
		return PR{
			PR:         &pr,
			AuthorInfo: Collaborator{GitHubName: pr.GetUser().GetName()},
		}
	}
	return PR{
		PR:         &pr,
		AuthorInfo: getAuthorInfo(pr, slackUserIdByGitHubUsername),
		Approvers:  getCollaborators(pr, pr.ApprovedByUsers, slackUserIdByGitHubUsername),
		Commenters: getCollaborators(pr, pr.CommentedByUsers, slackUserIdByGitHubUsername),
	}
}

func getAuthorInfo(pr githubclient.PR, slackUserIdByGitHubUsername *map[string]string) Collaborator {
	authorInfo := Collaborator{
		GitHubName: pr.GetAuthorNameOrUsername(),
	}
	if slackUserIdByGitHubUsername == nil || len(*slackUserIdByGitHubUsername) == 0 {
		return authorInfo
	}
	authorInfo.SlackUserID = (*slackUserIdByGitHubUsername)[pr.GetUsername()]
	return authorInfo
}

func getCollaborators(
	pr githubclient.PR,
	usernames []string,
	slackUserIdByGitHubUsername *map[string]string,
) []Collaborator {
	slackUserIds := make([]Collaborator, len(usernames))
	for i, username := range usernames {
		slackUserIds[i] = Collaborator{
			GitHubName:  pr.GetAuthorNameOrUsername(),
			SlackUserID: (*slackUserIdByGitHubUsername)[username],
		}
	}
	return slackUserIds
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
