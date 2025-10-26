// Package messagecontent structures PR data and configuration into content
// ready for message formatting. It handles text templating, PR grouping,
// and message content preparation.
package messagecontent

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/internal/models"
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
	switch {
	case len(openPRs) == 0:
		return Content{
			SummaryText: contentInputs.NoPRsMessage,
		}
	case contentInputs.GroupByRepository:
		return Content{
			SummaryText:            getSummaryText(len(openPRs)),
			PRsGroupedByRepository: groupPRsByRepositories(openPRs),
			GroupedByRepository:    true,
		}
	default:
		return Content{
			SummaryText:         getSummaryText(len(openPRs)),
			PRListHeading:       formatListHeading(contentInputs.PRListHeading, len(openPRs)),
			PRs:                 openPRs,
			GroupedByRepository: false,
		}
	}
}

func groupPRsByRepositories(openPRs []prparser.PR) []PRsOfRepository {
	prsByRepo := make(map[string][]prparser.PR)
	repoMap := make(map[string]models.Repository)

	for _, pr := range openPRs {
		repoKey := pr.Repository.GetPath()
		prsByRepo[repoKey] = append(prsByRepo[repoKey], pr)
		repoMap[repoKey] = pr.Repository
	}

	var repoKeys []string
	for repoKey := range prsByRepo {
		repoKeys = append(repoKeys, repoKey)
	}
	sort.Strings(repoKeys)

	return utilities.Map(repoKeys, func(repoKey string) PRsOfRepository {
		repo := repoMap[repoKey]
		return PRsOfRepository{
			HeadingPrefix:       "Open PRs in ",
			RepositoryLinkLabel: repo.GetPath(),
			RepositoryLink:      fmt.Sprintf("https://github.com/%s/pulls", repo.GetPath()),
			PRs:                 prsByRepo[repoKey],
		}
	})
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
