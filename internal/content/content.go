package content

import (
	"fmt"
	"math"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/parser"
)

type Content struct {
	SummaryText       string
	MainListHeading   string
	MainList          []parser.PR
	OldPRsListHeading string
	OldPRsList        []parser.PR
	NoPRsText         string
}

func (c Content) GetPRCount() int16 {
	return int16(len(c.MainList) + len(c.OldPRsList))
}

func (c Content) HasPRs() bool {
	return c.GetPRCount() > 0
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
	PRs     []parser.PR
}

func getNewAndOldPRs(openPRs []parser.PR, oldPRThresholdHours int) ([]parser.PR, []parser.PR) {
	mainList := []parser.PR{}
	oldPRsList := []parser.PR{}

	for _, pr := range openPRs {
		if pr.GetCreatedAt().After(time.Now().Add(-time.Duration(oldPRThresholdHours) * time.Hour)) {
			mainList = append(mainList, pr)
		} else {
			oldPRsList = append(oldPRsList, pr)
		}
	}
	return mainList, oldPRsList
}

func GetContent(openPRs []parser.PR, oldPRThresholdHours *int) Content {
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
			MainList:        openPRs,
		}
	default:
		newPRs, oldPRs := getNewAndOldPRs(openPRs, *oldPRThresholdHours)
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
