// Package messagebuilder constructs Slack Block Kit messages for PR reminders.
// It transforms structured PR content into rich text blocks with formatting,
// links, and user mentions suitable for Slack messaging.
package messagebuilder

import (
	"log"

	"github.com/hellej/pr-slack-reminder-action/internal/messagecontent"
	"github.com/hellej/pr-slack-reminder-action/internal/prparser"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
	"github.com/slack-go/slack"
)

// Slack API has limit of 50 blocks for PostMessage
// If the content is grouped by repository, each repository section uses 3 blocks (heading,
// PR list, spacing). To ensure that the last PR list is not cut off to only title, we set
// the limit to 50 blocks (16 repositories * 3 blocks each + 2 blocks for the last PR list
// which doesn't need spacing block after it).
const maximumBlocksInSlackMessage = 50

// The canvas link footer reserves two blocks rather than the one it takes: cutting the
// content at 49 would leave the 17th repository's heading with no PR list under it, whereas
// 48 ends on the 16th repository's spacing block, which reads as spacing before the footer.
const maximumBlocksWithCanvasLink = 48

func BuildMessage(content messagecontent.Content) (slack.Message, string) {
	var blocks []slack.Block

	if !content.HasPRs() {
		blocks = addNoPRsBlock(blocks, content.SummaryText)
		blocks = addCanvasLinkBlock(blocks, content.CanvasURL)
		return slack.NewBlockMessage(blocks...), content.SummaryText
	}

	if !content.GroupedByRepository {
		blocks = addPRListBLock(blocks, content.PRListHeading, content.PRs)
	} else {
		blocks = addRepositoryPRListBlocks(blocks, content.PRsGroupedByRepository)
	}

	// The link goes on after the truncation, so a large message keeps it instead of
	// dropping it as its first block over the limit.
	blocks = limitMaximumMessageSize(blocks, getMaximumBlocks(content.CanvasURL))
	blocks = addCanvasLinkBlock(blocks, content.CanvasURL)
	return slack.NewBlockMessage(blocks...), content.SummaryText
}

func getMaximumBlocks(canvasURL string) int {
	if canvasURL != "" {
		return maximumBlocksWithCanvasLink
	}
	return maximumBlocksInSlackMessage
}

func limitMaximumMessageSize(blocks []slack.Block, maximumBlocks int) []slack.Block {
	if len(blocks) > maximumBlocks {
		log.Printf(
			"Message content is too large (too many blocks: %v, dropping: %v)",
			len(blocks), len(blocks)-maximumBlocks,
		)
		blocks = blocks[:maximumBlocks]
	}
	return blocks
}

// A context block reads as a subdued footer rather than as another list row.
func addCanvasLinkBlock(blocks []slack.Block, canvasURL string) []slack.Block {
	if canvasURL == "" {
		return blocks
	}
	return append(blocks,
		slack.NewContextBlock("canvas_link",
			slack.NewTextBlockObject("mrkdwn", "<"+canvasURL+"|📋 PR tracker canvas>", false, false),
		),
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
			slack.NewRichTextSectionTextElement(pr.GetPRAgeDisplayText(), &slack.RichTextSectionTextStyle{Bold: true, Code: true}),
		)
	} else {
		ageElements = append(ageElements,
			slack.NewRichTextSectionTextElement(" "+pr.GetPRAgeDisplayText(), &slack.RichTextSectionTextStyle{Italic: true}),
		)
	}

	prItemElements := []slack.RichTextSectionElement{}

	linkStyle := &slack.RichTextSectionTextStyle{Bold: true, Strike: pr.IsClosedButNotMerged()}
	prItemElements = append(prItemElements,
		slack.NewRichTextSectionLinkElement(pr.GetHTMLURL(), pr.GetTitle(), linkStyle),
	)
	prItemElements = append(prItemElements, ageElements...)
	prItemElements = append(prItemElements,
		slack.NewRichTextSectionTextElement(" by ", &slack.RichTextSectionTextStyle{}),
		getUserNameElement(pr),
	)

	prItemElements = append(prItemElements, getReviewersElements(pr)...)

	if pr.IsMerged() {
		prItemElements = append(prItemElements,
			slack.NewRichTextSectionTextElement(" 🚀", &slack.RichTextSectionTextStyle{}),
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
	return utilities.Map(
		prparser.GetReviewersTextSegments(pr.Approvers, pr.Commenters),
		func(segment string) slack.RichTextSectionElement {
			return slack.NewRichTextSectionTextElement(segment, &slack.RichTextSectionTextStyle{})
		},
	)
}
