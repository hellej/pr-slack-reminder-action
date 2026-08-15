package githubclient

import (
	"slices"
	"strings"

	"github.com/hellej/pr-slack-reminder-action/internal/config"
)

func includePR(pr *PullRequest, filters config.Filters) bool {
	title := pr.GetTitle()
	for _, ignoredTerm := range filters.IgnoredTerms {
		if strings.Contains(title, ignoredTerm) {
			return false
		}
	}

	if len(filters.IgnoredLabels) > 0 {
		if slices.ContainsFunc(pr.Labels, func(label string) bool {
			return slices.Contains(filters.IgnoredLabels, label)
		}) {
			return false
		}
	}

	if len(filters.IgnoredAuthors) > 0 {
		if slices.Contains(filters.IgnoredAuthors, pr.Author.Login) {
			return false
		}
	}

	if len(filters.Labels) > 0 {
		if !slices.ContainsFunc(pr.Labels, func(label string) bool {
			return slices.Contains(filters.Labels, label)
		}) {
			return false
		}
	}

	if len(filters.Authors) > 0 {
		if !slices.Contains(filters.Authors, pr.Author.Login) {
			return false
		}
	}

	return true
}
