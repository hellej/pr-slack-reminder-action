package messagecontent

import (
	"fmt"
	"math"
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

func formatMainListHeading(heading string, prCount int) string {
	return strings.ReplaceAll(heading, "<pr_count>", strconv.Itoa(prCount))
}

func GetContent(openPRs []prparser.PR, contentInputs config.ContentInputs) Content {
	switch {
	case len(openPRs) == 0:
		return Content{
			SummaryText: contentInputs.NoPRsMessage,
		}
	case contentInputs.OldPRThresholdHours == nil:
		return Content{
			SummaryText:     fmt.Sprintf("%d open PRs are waiting for attention 👀", len(openPRs)),
			MainListHeading: formatMainListHeading(contentInputs.MainListHeading, len(openPRs)),
			MainList:        openPRs,
		}
	default:
		newPRs, oldPRs := getNewAndOldPRs(openPRs, *contentInputs.OldPRThresholdHours)
		content := Content{
			MainListHeading:   formatMainListHeading(contentInputs.MainListHeading, len(openPRs)),
			MainList:          newPRs,
			OldPRsListHeading: fmt.Sprintf("🚨 PRs older than %v", getOldPRsThresholdTimeLabel(*contentInputs.OldPRThresholdHours)),
			OldPRsList:        oldPRs,
		}
		content.SummaryText = fmt.Sprintf("%d open PRs are waiting for attention 👀", content.GetPRCount())
		return content
	}
}
