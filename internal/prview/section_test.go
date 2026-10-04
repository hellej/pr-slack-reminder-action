package prview_test

import (
	"slices"
	"testing"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/models"
	"github.com/hellej/pr-slack-reminder-action/internal/prview"
	"github.com/hellej/pr-slack-reminder-action/testhelpers"
)

func testPRByAuthor(number int, authorLogin string) prview.PR {
	return prview.PR{
		PR: &githubclient.PR{
			PullRequest: &githubclient.PullRequest{
				Number: number,
				Author: githubclient.Collaborator{Login: authorLogin},
			},
		},
	}
}

func TestRowsCollapsingPRsFromAuthors(t *testing.T) {
	defaultAuthors := []string{"dependabot[bot]", "renovate[bot]"}

	tests := []struct {
		name                   string
		prs                    []prview.PR
		collapsePRsFromAuthors []string
		expected               []string
	}{
		{
			name:                   "one PR by a listed author keeps its own row",
			prs:                    []prview.PR{testPRByAuthor(1, "alice"), testPRByAuthor(2, "dependabot[bot]")},
			collapsePRsFromAuthors: defaultAuthors,
			expected:               []string{"#1", "#2"},
		},
		{
			name: "two PRs by a listed author collapse after the other rows",
			prs: []prview.PR{
				testPRByAuthor(2, "dependabot[bot]"), testPRByAuthor(3, "dependabot[bot]"), testPRByAuthor(1, "alice"),
			},
			collapsePRsFromAuthors: defaultAuthors,
			expected:               []string{"#1", "dependabot[bot]: #2 #3"},
		},
		{
			// Renovate's PR leads the given order, so ordering by first appearance fails.
			name: "collapsed rows follow the order of the listed authors",
			prs: []prview.PR{
				testPRByAuthor(1, "renovate[bot]"), testPRByAuthor(2, "dependabot[bot]"),
				testPRByAuthor(3, "renovate[bot]"), testPRByAuthor(4, "dependabot[bot]"),
			},
			collapsePRsFromAuthors: defaultAuthors,
			expected:               []string{"dependabot[bot]: #2 #4", "renovate[bot]: #1 #3"},
		},
		{
			// The numbers run against the given order, so sorting by number fails.
			name: "interleaved PRs keep their given order, in the collapsed row and out of it",
			prs: []prview.PR{
				testPRByAuthor(9, "dependabot[bot]"), testPRByAuthor(5, "alice"),
				testPRByAuthor(7, "dependabot[bot]"), testPRByAuthor(3, "bob"),
				testPRByAuthor(4, "dependabot[bot]"),
			},
			collapsePRsFromAuthors: defaultAuthors,
			expected:               []string{"#5", "#3", "dependabot[bot]: #9 #7 #4"},
		},
		{
			name: "logins match exactly",
			prs: []prview.PR{
				testPRByAuthor(1, "dependabot"), testPRByAuthor(2, "dependabot"),
				testPRByAuthor(3, "Dependabot[bot]"), testPRByAuthor(4, "Dependabot[bot]"),
			},
			collapsePRsFromAuthors: defaultAuthors,
			expected:               []string{"#1", "#2", "#3", "#4"},
		},
		{
			name: "no listed authors keeps every PR in its own row",
			prs: []prview.PR{
				testPRByAuthor(1, "dependabot[bot]"), testPRByAuthor(2, "dependabot[bot]"),
			},
			collapsePRsFromAuthors: nil,
			expected:               []string{"#1", "#2"},
		},
		{
			name: "a login listed twice collapses into one row",
			prs: []prview.PR{
				testPRByAuthor(1, "dependabot[bot]"), testPRByAuthor(2, "dependabot[bot]"),
			},
			collapsePRsFromAuthors: []string{"dependabot[bot]", "dependabot[bot]"},
			expected:               []string{"dependabot[bot]: #1 #2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := prview.RowsCollapsingPRsFromAuthors(tt.prs, tt.collapsePRsFromAuthors)

			if got := testhelpers.DescribeRows(rows); !slices.Equal(got, tt.expected) {
				t.Errorf("expected rows %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestCollapsedRowGetAuthorLabel(t *testing.T) {
	tests := []struct {
		author   string
		expected string
	}{
		{author: "dependabot[bot]", expected: "dependabot"},
		{author: "renovate-bot", expected: "renovate-bot"},
		{author: "[bot]helper", expected: "[bot]helper"},
		{author: "my[bot]name[bot]", expected: "my[bot]name"},
	}

	for _, tt := range tests {
		t.Run(tt.author, func(t *testing.T) {
			if got := (prview.CollapsedRow{AuthorLogin: tt.author}).GetAuthorLabel(); got != tt.expected {
				t.Errorf("expected label %q, got %q", tt.expected, got)
			}
		})
	}
}

func testPRInRepositoryMerged(number int, repository string, merged bool) prview.PR {
	return prview.PR{
		PR: &githubclient.PR{
			PullRequest: &githubclient.PullRequest{Number: number, Merged: merged},
			Repository:  models.Repository{Owner: "test-org", Name: repository},
		},
	}
}

// The expected URLs are spelled out encoded, so an encoder mistake can't hide in the expectation.
func TestCollapsedRowGetSearchURL(t *testing.T) {
	tests := []struct {
		name        string
		authorLogin string
		prs         []prview.PR
		expected    string
	}{
		{
			name:        "open PRs of a bot in one repository",
			authorLogin: "dependabot[bot]",
			prs:         []prview.PR{testPRInRepositoryMerged(1, "app", false), testPRInRepositoryMerged(2, "app", false)},
			expected:    "https://github.com/test-org/app/pulls?q=is%3Apr+is%3Aopen+author%3Aapp%2Fdependabot",
		},
		{
			name:        "merged PRs of a bot in one repository",
			authorLogin: "dependabot[bot]",
			prs:         []prview.PR{testPRInRepositoryMerged(1, "app", true), testPRInRepositoryMerged(2, "app", true)},
			expected:    "https://github.com/test-org/app/pulls?q=is%3Apr+is%3Amerged+author%3Aapp%2Fdependabot",
		},
		{
			// infra leads the row though app sorts first, so the repositories follow the row.
			name:        "PRs spanning repositories",
			authorLogin: "renovate[bot]",
			prs: []prview.PR{
				testPRInRepositoryMerged(1, "infra", false), testPRInRepositoryMerged(2, "app", false),
				testPRInRepositoryMerged(3, "infra", false),
			},
			expected: "https://github.com/search?type=pullrequests&q=is%3Apr+is%3Aopen+author%3Aapp%2Frenovate+repo%3Atest-org%2Finfra+repo%3Atest-org%2Fapp",
		},
		{
			name:        "a plain login",
			authorLogin: "self_hosted_renovate",
			prs:         []prview.PR{testPRInRepositoryMerged(1, "app", false), testPRInRepositoryMerged(2, "app", false)},
			expected:    "https://github.com/test-org/app/pulls?q=is%3Apr+is%3Aopen+author%3Aself_hosted_renovate",
		},
		{
			name:        "a login with [bot] not at its end",
			authorLogin: "my[bot]name",
			prs:         []prview.PR{testPRInRepositoryMerged(1, "app", false), testPRInRepositoryMerged(2, "app", false)},
			expected:    "https://github.com/test-org/app/pulls?q=is%3Apr+is%3Aopen+author%3Amy%5Bbot%5Dname",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := prview.CollapsedRow{AuthorLogin: tt.authorLogin, PRs: tt.prs}
			if got := row.GetSearchURL(); got != tt.expected {
				t.Errorf("expected search URL\n%s\ngot\n%s", tt.expected, got)
			}
		})
	}
}
