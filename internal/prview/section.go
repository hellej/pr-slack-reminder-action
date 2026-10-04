package prview

import (
	"net/url"
	"slices"
	"strings"

	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

// How many PRs one author in collapse-prs-from-authors needs in a section, or a repository group,
// to get a collapsed row.
const MinPRsToCollapse = 2

// One section's rows, either as the flat list or as repository buckets, never both.
type PRSection struct {
	Rows   []Row
	Groups []RepositoryRows
}

func (section PRSection) HasRows() bool {
	return len(section.Rows) > 0 || len(section.Groups) > 0
}

type RepositoryRows struct {
	Repository models.Repository
	Rows       []Row
}

// Sealed: only this package's row kinds implement it, so a builder's type switch covers them all.
type Row interface {
	isRow()
}

func (PR) isRow() {}

type CollapsedRow struct {
	AuthorLogin string
	PRs         []PR
}

func (CollapsedRow) isRow() {}

// GitHub ends an app's login in "[bot]". See docs/third-party-facts.md § GitHub App bot logins end in `[bot]`, and GraphQL drops the suffix
func (row CollapsedRow) GetAuthorLabel() string {
	return strings.TrimSuffix(row.AuthorLogin, "[bot]")
}

// See prview.spec.md § Behaviour for the rule.
func RowsCollapsingPRsFromAuthors(prs []PR, collapsePRsFromAuthors []string) []Row {
	authorsToCollapse := utilities.UniqueFunc(collapsePRsFromAuthors, func(a, b string) bool { return a == b })
	collapsedRows := utilities.Filter(
		utilities.Map(authorsToCollapse, func(author string) CollapsedRow {
			return CollapsedRow{AuthorLogin: author, PRs: utilities.Filter(prs, func(pr PR) bool {
				return pr.getAuthorLogin() == author
			})}
		}),
		func(row CollapsedRow) bool { return len(row.PRs) >= MinPRsToCollapse },
	)
	authorsOfCollapsedRows := utilities.Map(collapsedRows, func(row CollapsedRow) string { return row.AuthorLogin })
	prsNotCollapsed := utilities.Filter(prs, func(pr PR) bool {
		return !slices.Contains(authorsOfCollapsedRows, pr.getAuthorLogin())
	})
	return slices.Concat(
		utilities.Map(prsNotCollapsed, func(pr PR) Row { return pr }),
		utilities.Map(collapsedRows, func(row CollapsedRow) Row { return row }),
	)
}

func (pr PR) getAuthorLogin() string {
	return pr.PullRequest.Author.Login
}

// The search lists every PR of the author in that state, not only the row's. See prview.spec.md § Doesn't Do
func (row CollapsedRow) GetSearchURL() string {
	qualifiers := []string{"is:pr", row.stateQualifier(), row.authorQualifier()}
	repositories := utilities.UniqueFunc(
		utilities.Map(row.PRs, func(pr PR) models.Repository { return pr.Repository }),
		func(a, b models.Repository) bool { return a == b },
	)
	if len(repositories) == 1 {
		return repositories[0].GetPullsURL() + "?q=" + url.QueryEscape(strings.Join(qualifiers, " "))
	}
	repositoryQualifiers := utilities.Map(repositories, func(repository models.Repository) string {
		return "repo:" + repository.GetPath()
	})
	query := strings.Join(slices.Concat(qualifiers, repositoryQualifiers), " ")
	return "https://github.com/search?type=pullrequests&q=" + url.QueryEscape(query)
}

func (row CollapsedRow) stateQualifier() string {
	if slices.ContainsFunc(row.PRs, func(pr PR) bool { return !pr.IsMerged() }) {
		return "is:open"
	}
	return "is:merged"
}

// GitHub search names an app author as app/<name>, not by its login. See docs/third-party-facts.md § GitHub search matches PRs created by an app with `author:app/USERNAME`
func (row CollapsedRow) authorQualifier() string {
	if strings.HasSuffix(row.AuthorLogin, "[bot]") {
		return "author:app/" + row.GetAuthorLabel()
	}
	return "author:" + row.AuthorLogin
}
