// Package messagebuilder constructs Slack Block Kit messages for PR reminders.
// It transforms structured PR content into rich text blocks with formatting,
// links, and user mentions suitable for Slack messaging.
package messagebuilder

import (
	"log"

	"github.com/hellej/pr-slack-reminder-action/internal/messagecontent"
	"github.com/hellej/pr-slack-reminder-action/internal/prparser"
	"github.com/slack-go/slack"
)

// Slack API has limit of 50 blocks for PostMessage
// If the content is grouped by repository, each repository section uses 3 blocks (heading,
// PR list, spacing). To ensure that the last PR list is not cut off to only title, we set
// the limit to 50 blocks (16 repositories * 3 blocks each + 2 blocks for the last PR list
// which doesn't need spacing block after it).
const maximumBlocksInSlackMessage = 50

func BuildMessage(content messagecontent.Content) (slack.Message, string) {
	var blocks []slack.Block

	if !content.HasPRs() {
		blocks = addNoPRsBlock(blocks, content.SummaryText)
		return slack.NewBlockMessage(blocks...), content.SummaryText
	}

	if !content.GroupedByRepository {
		blocks = addPRListBLock(blocks, content.PRListHeading, content.PRs)
	} else {
		blocks = addRepositoryPRListBlocks(blocks, content.PRsGroupedByRepository)
	}

	blocks = limitMaximumMessageSize(blocks)
	return slack.NewBlockMessage(blocks...), content.SummaryText
}

func limitMaximumMessageSize(blocks []slack.Block) []slack.Block {
	if len(blocks) > maximumBlocksInSlackMessage {
		log.Printf(
			"Message content is too large (too many blocks: %v, dropping: %v)",
			len(blocks), len(blocks)-maximumBlocksInSlackMessage,
		)
		blocks = blocks[:maximumBlocksInSlackMessage]
	}
	return blocks
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

func addRepositoryPRListBlocks(
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

	prItemElements := []slack.RichTextSectionElement{}

	linkText := pr.GetTitle()
	if pr.Prefix != "" {
		linkText = pr.Prefix + pr.GetTitle()
	}

	linkStyle := &slack.RichTextSectionTextStyle{Bold: true, Strike: pr.IsClosedButNotMerged()}
	prItemElements = append(prItemElements,
		slack.NewRichTextSectionLinkElement(pr.GetHTMLURL(), linkText, linkStyle),
	)
	prItemElements = append(prItemElements, ageElements...)
	prItemElements = append(prItemElements,
		slack.NewRichTextSectionTextElement(" by ", &slack.RichTextSectionTextStyle{}),
		getUserNameElement(pr),
	)

	prItemElements = append(prItemElements, getReviewersElements(pr)...)

	if pr.IsMerged() {
		prItemElements = append(prItemElements,
			slack.NewRichTextSectionTextElement(" 🔀", &slack.RichTextSectionTextStyle{}),
		)
	}

	return slack.NewRichTextSection(prItemElements...)
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
