// Package messagebuilder constructs Slack Block Kit messages for PR reminders.
// It transforms structured PR content into rich text blocks with formatting,
// links, and user mentions suitable for Slack messaging.
package messagebuilder

import (
	"github.com/hellej/pr-slack-reminder-action/internal/messagecontent"
	"github.com/hellej/pr-slack-reminder-action/internal/prparser"
	"github.com/slack-go/slack"
)

func BuildMessage(content messagecontent.Content) (slack.Message, string) {
	var blocks []slack.Block

	if !content.HasPRs() {
		blocks = addNoPRsBlock(blocks, content.SummaryText)
		return slack.NewBlockMessage(blocks...), content.SummaryText
	}

	if !content.GroupedByRepository {
		blocks = addPRListBLock(blocks, content.PRListHeading, content.PRs)
	} else {
		blocks = addGroupedPRsBlocks(blocks, content.PRsGroupedByRepository)
	}

	return slack.NewBlockMessage(blocks...), content.SummaryText
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

func addPRListBLock(blocks []slack.Block, heading string, prs []prparser.PR) []slack.Block {
	return append(blocks,
		slack.NewRichTextBlock("pr_list_heading",
			slack.NewRichTextSection(
				slack.NewRichTextSectionTextElement(heading, &slack.RichTextSectionTextStyle{Bold: true}),
			),
		),
		makePRListBlockWithID(prs, "open_prs"),
	)
}

func addGroupedPRsBlocks(
	blocks []slack.Block,
	prsGroupedByRepository []messagecontent.PRsOfRepository,
) []slack.Block {
	for idx, group := range prsGroupedByRepository {
		blocks = append(blocks,
			slack.NewRichTextBlock("pr_list_heading_"+group.RepositoryLinkLabel,
				slack.NewRichTextSection(
					slack.NewRichTextSectionTextElement(group.HeadingPrefix, &slack.RichTextSectionTextStyle{Bold: true}),
					slack.NewRichTextSectionLinkElement(
						group.RepositoryLink, group.RepositoryLinkLabel, &slack.RichTextSectionTextStyle{Bold: true},
					),
					slack.NewRichTextSectionTextElement(":", &slack.RichTextSectionTextStyle{Bold: true}),
				),
			),
		)
		blocks = append(blocks, makePRListBlockWithID(group.PRs, "open_prs_"+group.RepositoryLinkLabel))

		if idx < len(prsGroupedByRepository)-1 {
			// adding spacing block between repositories
			blocks = append(blocks,
				slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn", " ", false, false), nil, nil),
			)
		}
	}
	return blocks
}

func makePRListBlockWithID(openPRs []prparser.PR, blockID string) *slack.RichTextBlock {
	var prBlocks []slack.RichTextElement
	for _, pr := range openPRs {
		prBlocks = append(prBlocks, buildPRBulletPointBlock(pr))
	}
	return slack.NewRichTextBlock(
		blockID,
		slack.NewRichTextList(slack.RichTextListElementType("bullet"), 0,
			prBlocks...,
		),
	)
}

func buildPRBulletPointBlock(pr prparser.PR) slack.RichTextElement {
	var ageElements []slack.RichTextSectionElement

	if pr.IsOldPR {
		ageElements = append(ageElements,
			slack.NewRichTextSectionTextElement(" 🚨 ", &slack.RichTextSectionTextStyle{}),
			slack.NewRichTextSectionTextElement(pr.GetPRAgeText()+" old", &slack.RichTextSectionTextStyle{Bold: true, Code: true}),
		)
	} else {
		ageElements = append(ageElements,
			slack.NewRichTextSectionTextElement(" "+pr.GetPRAgeText()+" ago", &slack.RichTextSectionTextStyle{Italic: true}),
		)
	}

	titleAgeAndAuthorElements := []slack.RichTextSectionElement{}

	linkText := pr.GetTitle()
	if pr.Prefix != "" {
		linkText = pr.Prefix + pr.GetTitle()
	}

	titleAgeAndAuthorElements = append(titleAgeAndAuthorElements,
		slack.NewRichTextSectionLinkElement(pr.GetHTMLURL(), linkText, &slack.RichTextSectionTextStyle{Bold: true}),
	)
	titleAgeAndAuthorElements = append(titleAgeAndAuthorElements, ageElements...)
	titleAgeAndAuthorElements = append(titleAgeAndAuthorElements,
		slack.NewRichTextSectionTextElement(" by ", &slack.RichTextSectionTextStyle{}),
		getUserNameElement(pr),
	)

	return slack.NewRichTextSection(
		append(titleAgeAndAuthorElements, getReviewersElements(pr)...)...,
	)
}

func getUserNameElement(pr prparser.PR) slack.RichTextSectionElement {
	if pr.Author.SlackUserID != "" {
		return slack.NewRichTextSectionUserElement(
			pr.Author.SlackUserID, &slack.RichTextSectionTextStyle{},
		)
	}
	return slack.NewRichTextSectionTextElement(
		pr.Author.GetGitHubName(), &slack.RichTextSectionTextStyle{},
	)
}

func getReviewersElements(pr prparser.PR) []slack.RichTextSectionElement {
	var elements []slack.RichTextSectionElement
	approverCount := len(pr.Approvers)
	commenterCount := len(pr.Commenters)

	if approverCount == 0 && commenterCount == 0 {
		return elements
	}

	reviewerTextPrefix := " (💬 "
	if len(pr.Approvers) > 0 {
		reviewerTextPrefix = " (✅ "
	}
	elements = append(elements, slack.NewRichTextSectionTextElement(
		reviewerTextPrefix, &slack.RichTextSectionTextStyle{},
	))

	for idx, approver := range pr.Approvers {
		if idx > 0 {
			elements = append(elements, slack.NewRichTextSectionTextElement(
				", ", &slack.RichTextSectionTextStyle{},
			))
		}
		elements = append(elements, slack.NewRichTextSectionTextElement(
			approver.GetGitHubName(), &slack.RichTextSectionTextStyle{},
		))
	}

	if commenterCount == 0 {
		return append(elements, slack.NewRichTextSectionTextElement(
			")", &slack.RichTextSectionTextStyle{},
		))
	}

	if reviewerTextPrefix == " (✅ " {
		elements = append(elements, slack.NewRichTextSectionTextElement(
			" / 💬 ", &slack.RichTextSectionTextStyle{},
		))
	}

	for idx, commenter := range pr.Commenters {
		if idx > 0 {
			elements = append(elements, slack.NewRichTextSectionTextElement(
				", ", &slack.RichTextSectionTextStyle{},
			))
		}
		elements = append(elements, slack.NewRichTextSectionTextElement(
			commenter.GetGitHubName(), &slack.RichTextSectionTextStyle{},
		))
	}

	return append(elements, slack.NewRichTextSectionTextElement(
		")", &slack.RichTextSectionTextStyle{},
	))
}
