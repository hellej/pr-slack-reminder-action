package composer

import (
	"fmt"
	"math"
	"time"

	"github.com/google/go-github/v72/github"
	"github.com/slack-go/slack"
)

func getPRCreationTimeText(createdAt time.Time) string {
	duration := time.Since(createdAt)
	if duration.Hours() >= 24 {
		days := int(math.Round(duration.Hours())) / 24
		return fmt.Sprintf("%d days ago ", days)
	} else if duration.Hours() >= 1 {
		hours := int(math.Round(duration.Hours()))
		return fmt.Sprintf("%d hours ago ", hours)
	} else {
		minutes := int(math.Round(duration.Minutes()))
		return fmt.Sprintf("%d minutes ago ", minutes)
	}
}

func composePRBulletPointBlock(pr *github.PullRequest) slack.RichTextElement {
	var loginOrName string
	if pr.GetUser().GetName() != "" {
		loginOrName = pr.GetUser().GetName()
	} else {
		loginOrName = pr.GetUser().GetLogin()
	}

	return slack.NewRichTextSection(
		slack.NewRichTextSectionLinkElement(pr.GetHTMLURL(), pr.GetTitle(), &slack.RichTextSectionTextStyle{Bold: true}),
		slack.NewRichTextSectionTextElement(
			" ("+getPRCreationTimeText(pr.CreatedAt.Time)+"by "+loginOrName+")", &slack.RichTextSectionTextStyle{}),
	)
}

func composePRListBlock(openPRs []*github.PullRequest) *slack.RichTextBlock {
	var prBlocks []slack.RichTextElement
	for _, pr := range openPRs {
		prBlocks = append(prBlocks, composePRBulletPointBlock(pr))
	}
	return slack.NewRichTextBlock(
		"open_prs",
		slack.NewRichTextList(slack.RichTextListElementType("bullet"), 0,
			prBlocks...,
		),
	)

}

func ComposeMessage(openPRs []*github.PullRequest) (slack.Message, string) {
	var blocks []slack.Block

	if len(openPRs) > 0 {
		blocks = append(blocks, slack.NewHeaderBlock(
			slack.NewTextBlockObject("plain_text", "🚀 New PRs since 44 hours ago", false, false),
		),
			composePRListBlock(openPRs),
		)
	} else {
		blocks = append(blocks,
			slack.NewRichTextBlock("no_prs_block",
				slack.NewRichTextSection(
					slack.NewRichTextSectionTextElement("No new PRs since 44 hours ago", &slack.RichTextSectionTextStyle{}),
				),
			),
		)
	}

	return slack.NewBlockMessage(blocks...), "some message summary"
}
