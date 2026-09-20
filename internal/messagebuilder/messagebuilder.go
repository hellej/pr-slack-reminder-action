// Package messagebuilder constructs Slack Block Kit messages for PR reminders.
// It transforms structured PR content into rich text blocks with formatting,
// links, and user mentions suitable for Slack messaging.
package messagebuilder

import (
	"fmt"
	"log"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/messagecontent"
	"github.com/hellej/pr-slack-reminder-action/internal/prview"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
	"github.com/slack-go/slack"
)

// The section headings are this package's display text, not prview.PRNextAction values: those
// are identifiers. The merged heading says "recently" because the section shows the newest
// untracked merges only.
const (
	readyToMergeHeading     = "✅ Ready to merge"
	waitingForAuthorHeading = "💬 Waiting for author"
	waitingForReviewHeading = "👀 Waiting for review"
	mergedPRsHeading        = "🚀 Recently merged"
)

// Slack rejects a message of more than 50 blocks. The layout spends at most 9 of them: the
// no-open-PRs line, four section blocks with a spacing block between them, and the footer. So
// this is a safety net against an unforeseen layout, not a bound the content can reach.
const maximumBlocksInSlackMessage = 50

func BuildMessage(content messagecontent.Content) (slack.Message, string) {
	var blocks []slack.Block
	if content.NoOpenPRsText != "" {
		blocks = append(blocks, buildNoOpenPRsBlock(content.NoOpenPRsText))
	}
	blocks = append(blocks, buildSectionBlocks(content)...)
	blocks = limitMaximumMessageSize(blocks)
	blocks = append(blocks, buildFooterBlock(content.GeneratedAt))
	return slack.NewBlockMessage(blocks...), content.SummaryText
}

type section struct {
	blockID   string
	heading   string
	prs       messagecontent.PRSection
	renderRow func(prview.PR) slack.RichTextElement
}

// An empty section renders nothing at all, its heading included, so the spacing blocks fall
// between the sections that did render.
func buildSectionBlocks(content messagecontent.Content) []slack.Block {
	sections := []section{
		{"ready_to_merge", readyToMergeHeading, content.ReadyToMerge, buildOpenPRBulletPoint},
		{"waiting_for_author", waitingForAuthorHeading, content.WaitingForAuthor, buildOpenPRBulletPoint},
		{"waiting_for_review", waitingForReviewHeading, content.WaitingForReview, buildOpenPRBulletPoint},
		{"merged", mergedPRsHeading, content.Merged, buildMergedPRBulletPoint},
	}

	var blocks []slack.Block
	for _, section := range utilities.Filter(sections, sectionHasPRs) {
		if len(blocks) > 0 {
			blocks = append(blocks, buildSpacingBlock())
		}
		blocks = append(blocks, buildSectionBlock(section))
	}
	return blocks
}

func sectionHasPRs(section section) bool {
	return section.prs.HasPRs()
}

// A whole section goes in one block, so the block count stays independent of how many
// repositories the section spans.
func buildSectionBlock(section section) slack.Block {
	elements := []slack.RichTextElement{
		slack.NewRichTextSection(
			slack.NewRichTextSectionTextElement(section.heading, &slack.RichTextSectionTextStyle{Bold: true}),
		),
	}
	if len(section.prs.Groups) == 0 {
		elements = append(elements, buildPRList(section.prs.PRs, section.renderRow))
	}
	for _, group := range section.prs.Groups {
		elements = append(elements,
			buildRepositorySubHeading(group),
			buildPRList(group.PRs, section.renderRow),
		)
	}
	return slack.NewRichTextBlock("section_"+section.blockID, elements...)
}

// The section heading above already says what the rows are, so the sub-heading is the
// repository link alone, unbolded to sit under that heading.
func buildRepositorySubHeading(group messagecontent.PRsOfRepository) slack.RichTextElement {
	return slack.NewRichTextSection(
		slack.NewRichTextSectionLinkElement(
			group.RepositoryLink, group.RepositoryLinkLabel, &slack.RichTextSectionTextStyle{},
		),
		slack.NewRichTextSectionTextElement(":", &slack.RichTextSectionTextStyle{}),
	)
}

func buildPRList(
	prs []prview.PR, renderRow func(prview.PR) slack.RichTextElement,
) slack.RichTextElement {
	return slack.NewRichTextList(
		slack.RichTextListElementType("bullet"), 0, utilities.Map(prs, renderRow)...,
	)
}

func buildNoOpenPRsBlock(noOpenPRsText string) slack.Block {
	return slack.NewRichTextBlock("no_open_prs",
		slack.NewRichTextSection(
			slack.NewRichTextSectionTextElement(noOpenPRsText, &slack.RichTextSectionTextStyle{}),
		),
	)
}

func buildSpacingBlock() slack.Block {
	return slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn", " ", false, false), nil, nil)
}

// <!date^…> renders the time in each reader's own timezone, and the pipe fallback is what a
// client that cannot process it shows instead. A context block renders smaller and greyer than
// a rich_text line, so the footer doesn't read as a fifth section.
func buildFooterBlock(generatedAt time.Time) slack.Block {
	footerText := fmt.Sprintf(
		"_Live, updated <!date^%d^{time}|%s UTC>_",
		generatedAt.Unix(), generatedAt.UTC().Format("15:04"),
	)
	return slack.NewContextBlock("", slack.NewTextBlockObject("mrkdwn", footerText, false, false))
}

// The footer is appended after this, so it gets the last slot.
func limitMaximumMessageSize(blocks []slack.Block) []slack.Block {
	maximumContentBlocks := maximumBlocksInSlackMessage - 1
	if len(blocks) <= maximumContentBlocks {
		return blocks
	}
	log.Printf(
		"Message content is too large (too many blocks: %v, dropping: %v)",
		len(blocks), len(blocks)-maximumContentBlocks,
	)
	return blocks[:maximumContentBlocks]
}

func buildOpenPRBulletPoint(pr prview.PR) slack.RichTextElement {
	elements := []slack.RichTextSectionElement{
		slack.NewRichTextSectionLinkElement(
			pr.GetHTMLURL(), pr.GetTitle(), &slack.RichTextSectionTextStyle{Bold: true},
		),
	}
	elements = append(elements, buildAgeElements(pr)...)
	elements = append(elements,
		slack.NewRichTextSectionTextElement(" by ", &slack.RichTextSectionTextStyle{}),
		getUserNameElement(pr),
	)
	return slack.NewRichTextSection(append(elements, getReviewersElements(pr)...)...)
}

// A merged row shows when it landed instead of its age, so it carries neither the old-PR marker
// nor the rocket the section heading already has. An unknown merge time drops that segment.
func buildMergedPRBulletPoint(pr prview.PR) slack.RichTextElement {
	elements := []slack.RichTextSectionElement{
		slack.NewRichTextSectionLinkElement(
			pr.GetHTMLURL(), pr.GetTitle(), &slack.RichTextSectionTextStyle{Bold: true},
		),
	}
	if mergedText := pr.GetMergedText(); mergedText != "" {
		elements = append(elements, slack.NewRichTextSectionTextElement(
			" "+mergedText, &slack.RichTextSectionTextStyle{Italic: true},
		))
	}
	elements = append(elements,
		slack.NewRichTextSectionTextElement(" by ", &slack.RichTextSectionTextStyle{}),
		getUserNameElement(pr),
	)
	return slack.NewRichTextSection(append(elements, getReviewersElements(pr)...)...)
}

func buildAgeElements(pr prview.PR) []slack.RichTextSectionElement {
	if !pr.IsOldPR {
		return []slack.RichTextSectionElement{
			slack.NewRichTextSectionTextElement(
				" "+pr.GetPRAgeDisplayText(), &slack.RichTextSectionTextStyle{Italic: true},
			),
		}
	}
	return []slack.RichTextSectionElement{
		slack.NewRichTextSectionTextElement(" 🚨 ", &slack.RichTextSectionTextStyle{}),
		slack.NewRichTextSectionTextElement(
			pr.GetPRAgeDisplayText(), &slack.RichTextSectionTextStyle{Bold: true, Code: true},
		),
	}
}

func getUserNameElement(pr prview.PR) slack.RichTextSectionElement {
	if pr.Author.SlackUserID != "" {
		return slack.NewRichTextSectionUserElement(
			pr.Author.SlackUserID, &slack.RichTextSectionTextStyle{},
		)
	}
	return slack.NewRichTextSectionTextElement(
		pr.Author.GetGitHubName(), &slack.RichTextSectionTextStyle{},
	)
}

func getReviewersElements(pr prview.PR) []slack.RichTextSectionElement {
	return utilities.Map(
		prview.GetReviewersTextSegments(pr.Approvers, pr.Commenters),
		func(segment string) slack.RichTextSectionElement {
			return slack.NewRichTextSectionTextElement(segment, &slack.RichTextSectionTextStyle{})
		},
	)
}
