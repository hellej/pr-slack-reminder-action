// Package messagecontent structures PR data and configuration into content
// ready for message formatting. It handles text templating, PR grouping,
// and message content preparation.
package messagecontent

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/internal/prparser"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

type Content struct {
	SummaryText            string
	PRListHeading          string
	PRs                    []prparser.PR
	GroupedByRepository    bool
	PRsGroupedByRepository []PRsOfRepository
}

func (c Content) HasPRs() bool {
	return len(c.PRs) > 0 || len(c.PRsGroupedByRepository) > 0
}

type PRsOfRepository struct {
	HeadingPrefix       string
	RepositoryLinkLabel string
	RepositoryLink      string
	PRs                 []prparser.PR
}

func GetContent(openPRs []prparser.PR, contentInputs config.ContentInputs) Content {
	sortedOpenPRs := prparser.SortPRsOldestToNewest(openPRs)

	switch {
	case len(sortedOpenPRs) == 0:
		return Content{
			SummaryText: contentInputs.NoPRsMessage,
		}
	case contentInputs.GroupByRepository:
		return Content{
			SummaryText:            getSummaryText(len(sortedOpenPRs)),
			PRsGroupedByRepository: groupPRsByRepositories(sortedOpenPRs),
			GroupedByRepository:    true,
		}
	default:
		return Content{
			SummaryText:         getSummaryText(len(sortedOpenPRs)),
			PRListHeading:       formatListHeading(contentInputs.PRListHeading, len(sortedOpenPRs)),
			PRs:                 sortedOpenPRs,
			GroupedByRepository: false,
		}
	}
}

func groupPRsByRepositories(openPRs []prparser.PR) []PRsOfRepository {
	return utilities.Map(
		prparser.GroupPRsByRepositories(openPRs),
		func(group prparser.RepositoryPRs) PRsOfRepository {
			return PRsOfRepository{
				HeadingPrefix:       "Open PRs in ",
				RepositoryLinkLabel: group.Repository.GetPath(),
				RepositoryLink:      group.Repository.GetPullsURL(),
				PRs:                 group.PRs,
			}
		},
	)
}

func getSummaryText(prCount int) string {
	if prCount == 1 {
		return "1 open PR is waiting for attention 👀"
	}
	return fmt.Sprintf("%d open PRs are waiting for attention 👀", prCount)
}

func formatListHeading(heading string, prCount int) string {
	return strings.ReplaceAll(heading, "<pr_count>", strconv.Itoa(prCount))
}
