// Package messagebuilder constructs Slack Block Kit messages for PR reminders.
// It transforms structured PR content into rich text blocks with formatting,
// links, and user mentions suitable for Slack messaging.
package messagebuilder

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
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

// Slack rejects a message of more than 50 blocks. Grouped by repository, a section spends
// 2 × repositories of them: one block per repository, one spacing block between each pair, and
// the section heading.
const maximumBlocksInSlackMessage = 50

const slotsForFooterOrStalenessWarning = 1

const updateTimeFooterBlockID = "update_time_footer"

func BuildMessageToPost(content messagecontent.Content) (slack.Message, string) {
	return slack.NewBlockMessage(buildContentBlocks(content)...), content.SummaryText
}

func BuildMessageWithUpdateTimeFooter(content messagecontent.Content) (slack.Message, string) {
	blocks := append(buildContentBlocks(content), buildUpdateTimeFooterBlock(content.GeneratedAt))
	return slack.NewBlockMessage(blocks...), content.SummaryText
}

func buildContentBlocks(content messagecontent.Content) []slack.Block {
	var blocks []slack.Block
	if content.NoOpenPRsText != "" {
		blocks = append(blocks, buildNoOpenPRsBlock(content.NoOpenPRsText))
	}
	blocks = append(blocks, buildSectionBlocks(content)...)
	return limitMaximumMessageSize(blocks)
}

type section struct {
	blockIDFragment string
	heading         string
	prs             messagecontent.PRSection
	renderRow       func(prview.PR) slack.RichTextElement
}

func buildSectionBlocks(content messagecontent.Content) []slack.Block {
	sections := []section{
		{"ready_to_merge", readyToMergeHeading, content.ReadyToMerge, buildOpenPRBulletPoint},
		{"waiting_for_author", waitingForAuthorHeading, content.WaitingForAuthor, buildOpenPRBulletPoint},
		{"waiting_for_review", waitingForReviewHeading, content.WaitingForReview, buildOpenPRBulletPoint},
		{"merged", mergedPRsHeading, content.Merged, buildMergedPRBulletPoint},
	}

	var blocks []slack.Block
	for _, section := range utilities.Filter(sections, sectionHasPRs) {
		blocks = append(blocks, buildSectionHeadingBlock(section))
		blocks = append(blocks, buildSectionContentBlocks(section)...)
	}
	return blocks
}

func sectionHasPRs(section section) bool {
	return section.prs.HasPRs()
}

// A header block is the only message text larger than bold, and it carries its own vertical
// padding, so nothing else separates the sections.
func buildSectionHeadingBlock(section section) slack.Block {
	return slack.NewHeaderBlock(
		slack.NewTextBlockObject("plain_text", section.heading, true, false),
		slack.HeaderBlockOptionBlockID("heading_"+section.blockIDFragment),
		slack.HeaderBlockOptionLevel(2),
	)
}

// A spacing block cannot sit inside a rich_text block, so each repository takes a block of its
// own and the spacing blocks go between them.
func buildSectionContentBlocks(section section) []slack.Block {
	if len(section.prs.Groups) == 0 {
		return []slack.Block{
			buildPRListBlock("section_"+section.blockIDFragment, section.prs.PRs, section.renderRow),
		}
	}
	var blocks []slack.Block
	for repositoryPosition, group := range section.prs.Groups {
		if repositoryPosition > 0 {
			blocks = append(blocks, buildSpacingBlock())
		}
		// The repository's position identifies it, not its path: whether a block_id may hold
		// the path's "/" is not documented.
		blockID := fmt.Sprintf("section_%s_repository_%d", section.blockIDFragment, repositoryPosition+1)
		blocks = append(blocks, buildRepositoryBlock(blockID, group, section.renderRow))
	}
	return blocks
}

func buildRepositoryBlock(
	blockID string, group messagecontent.PRsOfRepository,
	renderRow func(prview.PR) slack.RichTextElement,
) slack.Block {
	subHeading := slack.NewRichTextSection(
		slack.NewRichTextSectionLinkElement(
			group.RepositoryPullsURL, group.RepositoryName, &slack.RichTextSectionTextStyle{Bold: true},
		),
	)
	return slack.NewRichTextBlock(blockID, subHeading, slack.NewRichTextList(
		slack.RichTextListElementType("bullet"), 0, utilities.Map(group.PRs, renderRow)...,
	))
}

func buildSpacingBlock() slack.Block {
	return slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn", " ", false, false), nil, nil)
}

func buildPRListBlock(
	blockID string, prs []prview.PR, renderRow func(prview.PR) slack.RichTextElement,
) slack.Block {
	return slack.NewRichTextBlock(blockID, slack.NewRichTextList(
		slack.RichTextListElementType("bullet"), 0, utilities.Map(prs, renderRow)...,
	))
}

func buildNoOpenPRsBlock(noOpenPRsText string) slack.Block {
	return slack.NewRichTextBlock("no_open_prs",
		slack.NewRichTextSection(
			slack.NewRichTextSectionTextElement(noOpenPRsText, &slack.RichTextSectionTextStyle{}),
		),
	)
}

// <!date^…> renders the time in each reader's own timezone, and the pipe fallback is what a
// client that cannot process it shows instead. A context block renders smaller and greyer than
// a rich_text line, so the footer doesn't read as a fifth section.
func buildUpdateTimeFooterBlock(generatedAt time.Time) slack.Block {
	footerText := fmt.Sprintf(
		"_Live, updated <!date^%d^{time}|%s UTC>_",
		generatedAt.Unix(), generatedAt.UTC().Format("15:04"),
	)
	return slack.NewContextBlock(updateTimeFooterBlockID, slack.NewTextBlockObject("mrkdwn", footerText, false, false))
}

// The stored blocks re-send as stored, so none has to survive a round trip through slack-go's
// block types. No cap: see messagebuilder.spec.md § Doesn't Do.
func BuildMessageMarkedStale(sentBlocks json.RawMessage, generatedAt time.Time) (slack.Message, error) {
	var sentBlockList []json.RawMessage
	if err := json.Unmarshal(sentBlocks, &sentBlockList); err != nil {
		return slack.Message{}, fmt.Errorf("failed to parse the sent blocks: %w", err)
	}
	if len(sentBlockList) == 0 {
		return slack.Message{}, errors.New("no sent blocks to mark stale")
	}
	contentBlocks, err := utilities.MapWithError(utilities.Filter(sentBlockList, isNotUpdateTimeFooter), blockFromJSON)
	if err != nil {
		return slack.Message{}, fmt.Errorf("failed to parse a sent block: %w", err)
	}
	markedMessageBlocks := slices.Concat([]slack.Block{buildStalenessWarningBlock(generatedAt)}, contentBlocks)
	return slack.NewBlockMessage(markedMessageBlocks...), nil
}

func isNotUpdateTimeFooter(sentBlock json.RawMessage) bool {
	var identifiedBlock struct {
		BlockID string `json:"block_id"`
	}
	// A block that does not parse is kept, for blockFromJSON to report
	_ = json.Unmarshal(sentBlock, &identifiedBlock)
	return identifiedBlock.BlockID != updateTimeFooterBlockID
}

// slack.BlockFromJSON keeps only the first block of an array (slack-go@v0.29.0/block_json.go).
func blockFromJSON(sentBlock json.RawMessage) (slack.Block, error) {
	return slack.BlockFromJSON(string(sentBlock))
}

// See messagebuilder.spec.md § Behaviour. A message marked stale is read days later, so the
// fallback names the date too.
func buildStalenessWarningBlock(generatedAt time.Time) slack.Block {
	stalenessWarningText := fmt.Sprintf(
		"_⚠️ Stale, updated <!date^%d^{date_pretty} at {time}|%s UTC>_",
		generatedAt.Unix(), generatedAt.UTC().Format("Jan 2 15:04"),
	)
	return slack.NewContextBlock("", slack.NewTextBlockObject("mrkdwn", stalenessWarningText, false, false))
}

func limitMaximumMessageSize(blocks []slack.Block) []slack.Block {
	maximumContentBlocks := maximumBlocksInSlackMessage - slotsForFooterOrStalenessWarning
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
