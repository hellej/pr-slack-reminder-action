package composer

import (
	"github.com/google/go-github/v72/github"
	"github.com/slack-go/slack"
)

func ComposeMessage(prs []*github.PullRequest) *slack.Message {
	prList := ""

	for _, pr := range prs {
		prList += "" + pr.GetHTMLURL() + "\n"
	}

	blocks := slack.Blocks{
		BlockSet: []slack.Block{
			slack.NewHeaderBlock(
				slack.NewTextBlockObject("plain_text", "🚀 New PRs since 44 hours ago", false, false),
			),
			slack.NewSectionBlock(
				slack.NewTextBlockObject("mrkdwn", prList, false, false),
				nil,
				nil,
			),
		},
	}

	return &slack.Message{
		Msg: slack.Msg{
			Blocks: blocks,
		},
	}
}
