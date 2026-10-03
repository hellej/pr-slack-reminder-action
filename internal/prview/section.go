package prview

import (
	"slices"
	"strings"

	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

// How many PRs one collapsed PR author needs in a section, or a repository group, to get a
// collapsed row.
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
func RowsCollapsingAuthors(prs []PR, collapsedPRAuthors []string) []Row {
	uniqueCollapsedPRAuthors := utilities.UniqueFunc(collapsedPRAuthors, func(a, b string) bool { return a == b })
	collapsedRows := utilities.Filter(
		utilities.Map(uniqueCollapsedPRAuthors, func(author string) CollapsedRow {
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
