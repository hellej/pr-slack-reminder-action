package parser

import (
	"fmt"
	"math"
	"time"

	"github.com/google/go-github/v72/github"
)

type PR struct {
	*github.PullRequest
}

func getPRAgeText(createdAt time.Time) string {
	duration := time.Since(createdAt)
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

func getPRUserDisplayName(pr *github.PullRequest) string {
	if pr.GetUser().GetName() != "" {
		return pr.GetUser().GetName()
	}
	return pr.GetUser().GetLogin()
}

func (pr PR) GetAgeUserInfoText() string {
	return fmt.Sprintf("%s by %s", getPRAgeText(pr.CreatedAt.Time), getPRUserDisplayName(pr.PullRequest))
}

func parsePR(pr *github.PullRequest) PR {
	return PR{
		PullRequest: pr,
	}
}

func ParsePRs(prs []*github.PullRequest) []PR {
	var parsedPRs []PR
	for _, pr := range prs {
		parsedPRs = append(parsedPRs, parsePR(pr))
	}
	return parsedPRs
}
