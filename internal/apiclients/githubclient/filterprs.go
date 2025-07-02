package githubclient

import "github.com/hellej/pr-slack-reminder-action/internal/config"

func filterPRs(
	prs []PR,
	globalFilters config.Filters,
	repositoryFilters map[string]config.Filters,
) []PR {
	filtered := make([]PR, 0, len(prs))
	for _, pr := range prs {
		repositoryFilters, ok := repositoryFilters[pr.Repository]
		if ok {
			if pr.isMatch(repositoryFilters) {
				filtered = append(filtered, pr)
			} else {
				continue
			}
		}
		if pr.isMatch(globalFilters) {
			filtered = append(filtered, pr)
		}
	}
	return filtered
}
