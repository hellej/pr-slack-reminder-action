package githubclient

import "github.com/hellej/pr-slack-reminder-action/internal/config"

func filterPRs(prs []PR, filters config.Filters) []PR {
	filtered := make([]PR, 0, len(prs))
	for _, pr := range prs {
		if pr.isMatch(filters) {
			filtered = append(filtered, pr)
		}
	}
	return filtered
}
