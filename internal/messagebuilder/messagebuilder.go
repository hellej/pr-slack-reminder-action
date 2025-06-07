package messagebuilder

import (
	"github.com/hellej/pr-slack-reminder-action/internal/messagecontent"
	"github.com/hellej/pr-slack-reminder-action/internal/prparser"
	"github.com/slack-go/slack"
)

func getUserNameElement(pr prparser.PR) slack.RichTextSectionElement {
	authorSlackUserID, ok := pr.GetAuthorSlackUserId()
	if ok {
		return slack.NewRichTextSectionUserElement(
			authorSlackUserID, &slack.RichTextSectionTextStyle{},
		)
	}
	return slack.NewRichTextSectionTextElement(
		pr.GetPRUserDisplayName(), &slack.RichTextSectionTextStyle{},
	)
}

func buildPRBulletPointBlock(pr prparser.PR) slack.RichTextElement {
	return slack.NewRichTextSection(
		slack.NewRichTextSectionLinkElement(pr.GetHTMLURL(), pr.GetTitle(), &slack.RichTextSectionTextStyle{Bold: true}),
		slack.NewRichTextSectionTextElement(
			" "+pr.GetPRAgeText(), &slack.RichTextSectionTextStyle{}),
		slack.NewRichTextSectionTextElement(
			" by ", &slack.RichTextSectionTextStyle{}),
		getUserNameElement(pr),
	)
}

func makePRListBlock(openPRs []prparser.PR) *slack.RichTextBlock {
	var prBlocks []slack.RichTextElement
	for _, pr := range openPRs {
		prBlocks = append(prBlocks, buildPRBulletPointBlock(pr))
	}
	return slack.NewRichTextBlock(
		"open_prs",
		slack.NewRichTextList(slack.RichTextListElementType("bullet"), 0,
			prBlocks...,
		),
	)
}

func addPRListBLock(blocks []slack.Block, heading string, prs []prparser.PR) []slack.Block {
	return append(blocks, slack.NewHeaderBlock(
		slack.NewTextBlockObject("plain_text", heading, false, false),
	),
		makePRListBlock(prs),
	)
}

func addNoPRsBlock(blocks []slack.Block, noPRsText string) []slack.Block {
	return append(blocks,
		slack.NewRichTextBlock("no_prs_block",
			slack.NewRichTextSection(
				slack.NewRichTextSectionTextElement(noPRsText, &slack.RichTextSectionTextStyle{}),
			),
		),
	)
}

func BuildMessage(content messagecontent.Content) (slack.Message, string) {
	var blocks []slack.Block

	if !content.HasPRs() {
		blocks = addNoPRsBlock(blocks, content.SummaryText)
		return slack.NewBlockMessage(blocks...), content.SummaryText
	}

	if len(content.MainList) > 0 {
		blocks = addPRListBLock(blocks, content.MainListHeading, content.MainList)
	}

	if len(content.OldPRsList) > 0 {
		blocks = addPRListBLock(blocks, content.OldPRsListHeading, content.OldPRsList)
	}

	return slack.NewBlockMessage(blocks...), content.SummaryText
}
