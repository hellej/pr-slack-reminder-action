package prparser

import (
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/google/go-github/v72/github"
	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
)

type PR struct {
	*github.PullRequest
	// Slack user ID of the author or empty string if not available
	GetAuthorSlackUserId func() (string, bool)
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

// Returns the name of the PR author if available, otherwise the GitHub username
func (pr PR) GetPRUserDisplayName() string {
	if pr.GetUser().GetName() != "" {
		return pr.GetUser().GetName()
	}
	return pr.GetUser().GetLogin()
}

func ParsePRs(prs []githubclient.PR, slackUserIdByGitHubUsername *map[string]string) []PR {
	var parsedPRs []PR
	for _, pr := range prs {
		parsedPRs = append(parsedPRs, parsePR(pr, slackUserIdByGitHubUsername))
	}
	return sortPRsByCreatedAt(parsedPRs)
}

func parsePR(pr githubclient.PR, slackUserIdByGitHubUsername *map[string]string) PR {
	if slackUserIdByGitHubUsername == nil || len(*slackUserIdByGitHubUsername) == 0 {
		return PR{
			PullRequest: pr.PullRequest,
			GetAuthorSlackUserId: func() (string, bool) {
				return "", false
			},
		}
	}
	return PR{
		PullRequest:          pr.PullRequest,
		GetAuthorSlackUserId: getAuthorSlackUserId(pr, slackUserIdByGitHubUsername),
	}
}

func getAuthorSlackUserId(pr githubclient.PR, slackUserIdByGitHubUsername *map[string]string) func() (string, bool) {
	return func() (string, bool) {
		gitHubUsername := pr.GetUser().GetLogin()
		if gitHubUsername == "" {
			return "", false
		}
		slackUserId, ok := (*slackUserIdByGitHubUsername)[pr.GetUser().GetLogin()]
		if !ok {
			return "", false
		}
		return slackUserId, true
	}
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
