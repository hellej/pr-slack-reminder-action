package composer

import (
	"github.com/google/go-github/v72/github"
	"github.com/slack-go/slack"
)

func ComposeMessage(prs []*github.PullRequest) slack.Message {
	prList := ""

	for _, pr := range prs {
		prList += "" + pr.GetHTMLURL() + "\n"
	}

	blockMessage := slack.NewBlockMessage(
		slack.NewHeaderBlock(
			slack.NewTextBlockObject("plain_text", "🚀 New PRs since 44 hours ago", false, false),
		),
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", prList, false, false),
			nil,
			nil,
		),
	)

	return blockMessage
}
