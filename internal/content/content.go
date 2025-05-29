package content

import (
	"fmt"
	"math"
	"time"

	"github.com/google/go-github/v72/github"
)

type Content struct {
	SummaryText       string
	MainListHeading   string
	MainList          []PR
	OldPRsListHeading string
	OldPRsList        []PR
	NoPRsText         string
}

func (c Content) GetPRCount() int16 {
	return int16(len(c.MainList) + len(c.OldPRsList))
}

func (c Content) HasPRs() bool {
	return c.GetPRCount() > 0
}

type PR struct {
	*github.PullRequest
}

func (pr PR) GetAgeUserInfoText() string {
	return fmt.Sprintf("%s by %s", getPRAgeText(pr.CreatedAt.Time), getPRUserDisplayName(pr.PullRequest))

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

func parsePR(pr *github.PullRequest) PR {
	return PR{
		PullRequest: pr,
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

func getNewAndOldPRs(openPRs []PR, oldPRThresholdHours int) ([]PR, []PR) {
	mainList := []PR{}
	oldPRsList := []PR{}

	for _, pr := range openPRs {
		if pr.GetCreatedAt().After(time.Now().Add(-time.Duration(oldPRThresholdHours) * time.Hour)) {
			mainList = append(mainList, pr)
		} else {
			oldPRsList = append(oldPRsList, pr)
		}
	}
	return mainList, oldPRsList
}

func GetContent(openPRs []*github.PullRequest, oldPRThresholdHours *int) Content {
	allPRs := parsePRs(openPRs)

	switch {
	case len(openPRs) == 0:
		text := "No open PRs, happy coding! 🎉"
		return Content{
			SummaryText: text,
			NoPRsText:   text,
		}
	case oldPRThresholdHours == nil:
		return Content{
			SummaryText:     fmt.Sprintf("%d open PRs are waiting for attention 👀", len(openPRs)),
			MainListHeading: fmt.Sprintf("🚀 There are %d open PRs", len(openPRs)),
			MainList:        allPRs,
		}
	default:
		newPRs, oldPRs := getNewAndOldPRs(allPRs, *oldPRThresholdHours)
		content := Content{
			MainListHeading:   "🚀 New open PRs",
			MainList:          newPRs,
			OldPRsListHeading: fmt.Sprintf("🚨 Old PRs since %v ago", getOldPRsThresholdTimeLabel(*oldPRThresholdHours)),
			OldPRsList:        oldPRs,
		}
		content.SummaryText = fmt.Sprintf("%d open PRs are waiting for attention 👀", content.GetPRCount())
		return content
	}
}
