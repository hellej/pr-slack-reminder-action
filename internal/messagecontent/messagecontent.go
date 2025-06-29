package messagecontent

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/internal/prparser"
)

type Content struct {
	SummaryText       string
	MainListHeading   string
	MainList          []prparser.PR
	OldPRsListHeading string
	OldPRsList        []prparser.PR
}

func (c Content) GetPRCount() int {
	return len(c.MainList) + len(c.OldPRsList)
}

func (c Content) HasPRs() bool {
	return c.GetPRCount() > 0
}

type PRCategory struct {
	Heading string
	PRs     []prparser.PR
}

func getNewAndOldPRs(openPRs []prparser.PR, oldPRThresholdHours int) ([]prparser.PR, []prparser.PR) {
	mainList := []prparser.PR{}
	oldPRsList := []prparser.PR{}

	for _, pr := range openPRs {
		if pr.GetCreatedAt().After(time.Now().Add(-time.Duration(oldPRThresholdHours) * time.Hour)) {
			mainList = append(mainList, pr)
		} else {
			oldPRsList = append(oldPRsList, pr)
		}
	}
	return mainList, oldPRsList
}

func formatListHeading(heading string, prCount int) string {
	return strings.ReplaceAll(heading, "<pr_count>", strconv.Itoa(prCount))
}

func getSummaryText(prCount int) string {
	if prCount == 1 {
		return "1 open PR is waiting for attention 👀"
	}
	return fmt.Sprintf("%d open PRs are waiting for attention 👀", prCount)
}

func GetContent(openPRs []prparser.PR, contentInputs config.ContentInputs) Content {
	switch {
	case len(openPRs) == 0:
		return Content{
			SummaryText: contentInputs.NoPRsMessage,
		}
	case contentInputs.OldPRThresholdHours == nil:
		return Content{
			SummaryText:     getSummaryText(len(openPRs)),
			MainListHeading: formatListHeading(contentInputs.MainListHeading, len(openPRs)),
			MainList:        openPRs,
		}
	default:
		newPRs, oldPRs := getNewAndOldPRs(openPRs, *contentInputs.OldPRThresholdHours)
		return Content{
			SummaryText:       getSummaryText(len(newPRs) + len(oldPRs)),
			MainListHeading:   formatListHeading(contentInputs.MainListHeading, len(openPRs)),
			MainList:          newPRs,
			OldPRsListHeading: formatListHeading(contentInputs.OldPRsListHeading, len(oldPRs)),
			OldPRsList:        oldPRs,
		}
	}
}
