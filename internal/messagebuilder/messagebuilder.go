package messagebuilder

import (
	"github.com/hellej/pr-slack-reminder-action/internal/messagecontent"
	"github.com/hellej/pr-slack-reminder-action/internal/prparser"
	"github.com/slack-go/slack"
)

func getUserNameElement(pr prparser.PR) slack.RichTextSectionElement {

	if pr.AuthorInfo.SlackUserID != "" {
		return slack.NewRichTextSectionUserElement(
			pr.AuthorInfo.SlackUserID, &slack.RichTextSectionTextStyle{},
		)
	}
	return slack.NewRichTextSectionTextElement(
		pr.GetAuthorNameOrUsername(), &slack.RichTextSectionTextStyle{},
	)
}

func getReviewersElements(pr prparser.PR) []slack.RichTextSectionElement {
	var elements []slack.RichTextSectionElement
	approverCount := len(pr.Approvers)
	commenterCount := len(pr.Commenters)

	if approverCount == 0 && commenterCount == 0 {
		return append(
			elements, slack.NewRichTextSectionTextElement(
				" (no reviews)", &slack.RichTextSectionTextStyle{},
			),
		)
	}

	reviewerTextPrefix := " (reviewed by "
	if len(pr.Approvers) > 0 {
		reviewerTextPrefix = " (approved by "
	}
	elements = append(elements, slack.NewRichTextSectionTextElement(
		reviewerTextPrefix, &slack.RichTextSectionTextStyle{},
	))

	for idx, approver := range pr.Approvers {
		elements = append(elements, slack.NewRichTextSectionTextElement(
			approver.GitHubName, &slack.RichTextSectionTextStyle{},
		))
		if idx > 0 && idx < approverCount-1 {
			elements = append(elements, slack.NewRichTextSectionTextElement(
				", ", &slack.RichTextSectionTextStyle{},
			))
		}
	}

	if commenterCount == 0 {
		return append(elements, slack.NewRichTextSectionTextElement(
			")", &slack.RichTextSectionTextStyle{},
		))
	}

	if reviewerTextPrefix == " (approved by " {
		elements = append(elements, slack.NewRichTextSectionTextElement(
			"- reviewed by ", &slack.RichTextSectionTextStyle{},
		))
	}

	for idx, commenter := range pr.Commenters {
		elements = append(elements, slack.NewRichTextSectionTextElement(
			commenter.GitHubName, &slack.RichTextSectionTextStyle{},
		))
		if idx > 0 && idx < commenterCount-1 {
			elements = append(elements, slack.NewRichTextSectionTextElement(
				", ", &slack.RichTextSectionTextStyle{},
			))
		}
	}

	return append(elements, slack.NewRichTextSectionTextElement(
		")", &slack.RichTextSectionTextStyle{},
	))
}

func buildPRBulletPointBlock(pr prparser.PR) slack.RichTextElement {
	titleAgeAndAuthorElements := []slack.RichTextSectionElement{
		slack.NewRichTextSectionLinkElement(pr.GetHTMLURL(), pr.GetTitle(), &slack.RichTextSectionTextStyle{Bold: true}),
		slack.NewRichTextSectionTextElement(
			" "+pr.GetPRAgeText(), &slack.RichTextSectionTextStyle{}),
		slack.NewRichTextSectionTextElement(
			" by ", &slack.RichTextSectionTextStyle{}),
		getUserNameElement(pr),
	}
	return slack.NewRichTextSection(
		append(titleAgeAndAuthorElements, getReviewersElements(pr)...)...,
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
