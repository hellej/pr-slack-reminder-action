package composer

import (
	"github.com/google/go-github/v72/github"
	"github.com/slack-go/slack"
)

func ComposeMessage(openPRs []*github.PullRequest) slack.Message {
	var blocks []slack.Block

	prList := ""
	for _, pr := range openPRs {
		prList += "" + pr.GetHTMLURL() + "\n"
	}

	if len(openPRs) > 0 {
		blocks = append(blocks, slack.NewHeaderBlock(
			slack.NewTextBlockObject("plain_text", "🚀 New PRs since 44 hours ago", false, false),
		),
			slack.NewSectionBlock(
				slack.NewTextBlockObject("mrkdwn", prList, false, false),
				nil,
				nil,
			))
	} else {
		blocks = append(blocks,
			slack.NewSectionBlock(
				slack.NewTextBlockObject("mrkdwn", "No open PRs since 44 hours ago", false, false),
				nil,
				nil,
			),
		)
	}

	blockMessage := slack.NewBlockMessage(blocks...)

	return blockMessage
}
