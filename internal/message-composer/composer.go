package composer

import "github.com/google/go-github/v72/github"

func ComposeMessage(prs []github.PullRequest) string {
	message := "💫 Open PRs:\n"

	for _, pr := range prs {
		message += pr.GetHTMLURL() + "\n"
	}

	return message

}
