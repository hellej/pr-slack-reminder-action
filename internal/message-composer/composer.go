package composer

import (
	"github.com/google/go-github/v72/github"
	"github.com/slack-go/slack"
)

func composePRBulletPointBlock(pr *github.PullRequest) *slack.RichTextSection {
	var loginOrName string
	if pr.GetUser().GetName() != "" {
		loginOrName = pr.GetUser().GetName()
	} else {
		loginOrName = pr.GetUser().GetLogin()
	}

	return slack.NewRichTextSection(
		slack.NewRichTextSectionLinkElement(pr.GetHTMLURL(), pr.GetTitle(), &slack.RichTextSectionTextStyle{Bold: true}),
		slack.NewRichTextSectionTextElement(
			" (by "+loginOrName+")", &slack.RichTextSectionTextStyle{}),
	)
}

func composePRListBlock(openPRs []*github.PullRequest) *slack.RichTextBlock {
	var prBlocks []slack.RichTextSection
	for _, pr := range openPRs {
		prBlocks = append(prBlocks, *composePRBulletPointBlock(pr))
	}

	return slack.NewRichTextBlock(
		"open_prs",
		slack.NewRichTextList(slack.RichTextListElementType("bullet"), 0,
			prBlocks[0], prBlocks[1],
		),
	)

}

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
			composePRListBlock(openPRs),
		)
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
