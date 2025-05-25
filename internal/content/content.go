package content

import (
	"fmt"
	"math"
	"time"

	"github.com/google/go-github/v72/github"
)

type Content struct {
	SummaryText       string
	NoPRsText         string
	MainListHeading   string
	MainList          []PR
	OldPRsListHeading string
	OldPRsList        []PR
}

type PR struct {
	github.PullRequest
	AgeUserInfoText string
}

func getPRAgeText(createdAt time.Time) string {
	duration := time.Since(createdAt)
	if duration.Hours() >= 24 {
		days := int(math.Round(duration.Hours())) / 24
		return fmt.Sprintf("%d days ago ", days)
	} else if duration.Hours() >= 1 {
		hours := int(math.Round(duration.Hours()))
		return fmt.Sprintf("%d hours ago ", hours)
	} else {
		minutes := int(math.Round(duration.Minutes()))
		return fmt.Sprintf("%d minutes ago ", minutes)
	}
}

func getPRUserDisplayName(pr *github.PullRequest) string {
	if pr.GetUser().GetName() != "" {
		return pr.GetUser().GetName()
	}
	return pr.GetUser().GetLogin()
}

func parsePR(pr *github.PullRequest) PR {
	return PR{
		PullRequest:     *pr,
		AgeUserInfoText: fmt.Sprintf("%s by %s", getPRAgeText(pr.CreatedAt.Time), getPRUserDisplayName(pr)),
	}
}

func parsePRs(prs []*github.PullRequest) []PR {
	var parsedPRs []PR
	for _, pr := range prs {
		parsedPRs = append(parsedPRs, parsePR(pr))
	}
	return parsedPRs
}

func getOldPRsThresholdTimeLabel(oldPRThresholdHours int) string {
	if oldPRThresholdHours < 24 {
		return fmt.Sprintf("%d hours", oldPRThresholdHours)
	}
	days := int(math.Round(float64(oldPRThresholdHours / 24)))
	return fmt.Sprintf("%d days", days)
}

type PRCategory struct {
	Heading string
	PRs     []PR
}

func getPRCategoryHeadings(oldPRThresholdHours int) (string, string) {
	timeThresholdLabel := getOldPRsThresholdTimeLabel(oldPRThresholdHours)
	mainHeading := "🚀 New PRs since " + timeThresholdLabel + " ago"
	oldPRsHeading := "⌛️ Old PRs since " + timeThresholdLabel + " ago"
	return mainHeading, oldPRsHeading

}

func getContentWithNewAndOldPRs(openPRs []PR, oldPRThresholdHours int) Content {
	mainHeading, oldPRsHeading := getPRCategoryHeadings(oldPRThresholdHours)
	content := Content{
		MainListHeading:   mainHeading,
		OldPRsListHeading: oldPRsHeading,
	}
	for _, pr := range openPRs {
		if pr.GetCreatedAt().After(time.Now().Add(-time.Duration(oldPRThresholdHours) * time.Hour)) {
			content.MainList = append(content.MainList, pr)
		} else {
			content.OldPRsList = append(content.OldPRsList, pr)
		}
	}
	return content
}

func GetContent(openPRs []*github.PullRequest, oldPRThresholdHours *int) Content {
	allPRs := parsePRs(openPRs)

	switch {
	case len(openPRs) == 0:
		return Content{
			NoPRsText: "No open PRs, happy coding! 🎉",
		}
	case oldPRThresholdHours == nil:
		return Content{
			SummaryText:     fmt.Sprintf("%d open PRs are waiting for attention", len(openPRs)),
			MainListHeading: fmt.Sprintf("There are %d open PRs 👀", len(openPRs)),
		}
	default:
		content := getContentWithNewAndOldPRs(allPRs, *oldPRThresholdHours)
		content.SummaryText = "%d open PRs are waiting for attention"
		return content
	}
}
