package prview_test

import (
	"slices"
	"testing"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
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

func TestRowsCollapsingAuthors(t *testing.T) {
	defaultAuthors := []string{"dependabot[bot]", "renovate[bot]"}

	tests := []struct {
		name               string
		prs                []prview.PR
		collapsedPRAuthors []string
		expected           []string
	}{
		{
			name:               "one PR by a collapsed PR author keeps its own row",
			prs:                []prview.PR{testPRByAuthor(1, "alice"), testPRByAuthor(2, "dependabot[bot]")},
			collapsedPRAuthors: defaultAuthors,
			expected:           []string{"#1", "#2"},
		},
		{
			name: "two PRs by a collapsed PR author collapse after the other rows",
			prs: []prview.PR{
				testPRByAuthor(2, "dependabot[bot]"), testPRByAuthor(3, "dependabot[bot]"), testPRByAuthor(1, "alice"),
			},
			collapsedPRAuthors: defaultAuthors,
			expected:           []string{"#1", "dependabot[bot]: #2 #3"},
		},
		{
			// Renovate's PR leads the given order, so ordering by first appearance fails.
			name: "collapsed rows follow the order of the collapsed PR authors",
			prs: []prview.PR{
				testPRByAuthor(1, "renovate[bot]"), testPRByAuthor(2, "dependabot[bot]"),
				testPRByAuthor(3, "renovate[bot]"), testPRByAuthor(4, "dependabot[bot]"),
			},
			collapsedPRAuthors: defaultAuthors,
			expected:           []string{"dependabot[bot]: #2 #4", "renovate[bot]: #1 #3"},
		},
		{
			// The numbers run against the given order, so sorting by number fails.
			name: "interleaved PRs keep their given order, in the collapsed row and out of it",
			prs: []prview.PR{
				testPRByAuthor(9, "dependabot[bot]"), testPRByAuthor(5, "alice"),
				testPRByAuthor(7, "dependabot[bot]"), testPRByAuthor(3, "bob"),
				testPRByAuthor(4, "dependabot[bot]"),
			},
			collapsedPRAuthors: defaultAuthors,
			expected:           []string{"#5", "#3", "dependabot[bot]: #9 #7 #4"},
		},
		{
			name: "logins match exactly",
			prs: []prview.PR{
				testPRByAuthor(1, "dependabot"), testPRByAuthor(2, "dependabot"),
				testPRByAuthor(3, "Dependabot[bot]"), testPRByAuthor(4, "Dependabot[bot]"),
			},
			collapsedPRAuthors: defaultAuthors,
			expected:           []string{"#1", "#2", "#3", "#4"},
		},
		{
			name: "no collapsed PR authors keeps every PR in its own row",
			prs: []prview.PR{
				testPRByAuthor(1, "dependabot[bot]"), testPRByAuthor(2, "dependabot[bot]"),
			},
			collapsedPRAuthors: nil,
			expected:           []string{"#1", "#2"},
		},
		{
			name: "a login listed twice collapses into one row",
			prs: []prview.PR{
				testPRByAuthor(1, "dependabot[bot]"), testPRByAuthor(2, "dependabot[bot]"),
			},
			collapsedPRAuthors: []string{"dependabot[bot]", "dependabot[bot]"},
			expected:           []string{"dependabot[bot]: #1 #2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := prview.RowsCollapsingAuthors(tt.prs, tt.collapsedPRAuthors)

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
