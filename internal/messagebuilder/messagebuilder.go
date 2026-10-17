// Package messagebuilder constructs Slack Block Kit messages for PR reminders.
// It transforms structured PR content into rich text blocks with formatting,
// links, and user mentions suitable for Slack messaging.
package messagebuilder

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/mergestats"
	"github.com/hellej/pr-slack-reminder-action/internal/messagecontent"
	"github.com/hellej/pr-slack-reminder-action/internal/models"
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

// Slack rejects a message of more than 50 blocks. See messagebuilder.spec.md § Oddities for
// when grouping by repository reaches it.
const maximumBlocksInSlackMessage = 50

const (
	slotsForMergeStatsLine           = 1
	slotsForFooterOrStalenessWarning = 1
)

const updateTimeFooterBlockID = "update_time_footer"

const mergeStatsLineBlockID = "merge_stats_line"

func BuildMessageToPost(content messagecontent.Content) (slack.Message, string) {
	return slack.NewBlockMessage(buildMessageBodyBlocks(content)...), content.SummaryText
}

func BuildMessageWithUpdateTimeFooter(content messagecontent.Content) (slack.Message, string) {
	blocks := append(buildMessageBodyBlocks(content), buildUpdateTimeFooterBlock(content.GeneratedAt))
	return slack.NewBlockMessage(blocks...), content.SummaryText
}

func buildMessageBodyBlocks(content messagecontent.Content) []slack.Block {
	blocks := buildContentBlocks(content)
	if content.MergeStats != nil {
		blocks = append(blocks, buildMergeStatsLineBlock(*content.MergeStats, content.MergeStatsWeekBefore))
	}
	return blocks
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
	blockIDFragment    string
	heading            string
	prSection          prview.PRSection
	buildPRBulletPoint func(prview.PR) slack.RichTextElement
}

func buildSectionBlocks(content messagecontent.Content) []slack.Block {
	sections := []section{
		{"waiting_for_review", waitingForReviewHeading, content.WaitingForReview, buildOpenPRBulletPoint},
		{"waiting_for_author", waitingForAuthorHeading, content.WaitingForAuthor, buildOpenPRBulletPoint},
		{"ready_to_merge", readyToMergeHeading, content.ReadyToMerge, buildOpenPRBulletPoint},
		{"merged", mergedPRsHeading, content.Merged, buildMergedPRBulletPoint},
	}

	var blocks []slack.Block
	for _, section := range utilities.Filter(sections, sectionHasRows) {
		blocks = append(blocks, buildSectionHeadingBlock(section))
		blocks = append(blocks, buildSectionContentBlocks(section)...)
	}
	return blocks
}

func sectionHasRows(section section) bool {
	return section.prSection.HasRows()
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

func buildSectionContentBlocks(section section) []slack.Block {
	if len(section.prSection.Groups) == 0 {
		return []slack.Block{
			slack.NewRichTextBlock("section_"+section.blockIDFragment, buildRowList(section.prSection.Rows, section)),
		}
	}
	var blocks []slack.Block
	for repositoryPosition, group := range section.prSection.Groups {
		// The repository's position identifies it, not its path: whether a block_id may hold
		// the path's "/" is not documented.
		blockID := fmt.Sprintf("section_%s_repository_%d", section.blockIDFragment, repositoryPosition+1)
		blocks = append(blocks,
			buildRepositorySubHeadingBlock(blockID+"_sub_heading", group.Repository),
			slack.NewRichTextBlock(blockID, buildRowList(group.Rows, section)),
		)
	}
	return blocks
}

// A context block renders smaller than a rich_text line, so the sub-heading doesn't
// compete with the bold PR titles. GitHub repository names hold none of the characters mrkdwn
// escapes: &, < and >.
func buildRepositorySubHeadingBlock(blockID string, repository models.Repository) slack.Block {
	subHeadingText := fmt.Sprintf("*<%s|%s>*", repository.GetPullsURL(), repository.Name)
	return slack.NewContextBlock(blockID, slack.NewTextBlockObject("mrkdwn", subHeadingText, false, false))
}

func buildRowList(rows []prview.Row, section section) *slack.RichTextList {
	return slack.NewRichTextList(
		slack.RichTextListElementType("bullet"), 0,
		utilities.Map(rows, func(row prview.Row) slack.RichTextElement { return buildBulletPoint(row, section) })...,
	)
}

func buildBulletPoint(row prview.Row, section section) slack.RichTextElement {
	switch row := row.(type) {
	case prview.CollapsedRow:
		return buildCollapsedRowBulletPoint(row)
	default:
		return section.buildPRBulletPoint(row.(prview.PR))
	}
}

func buildCollapsedRowBulletPoint(row prview.CollapsedRow) slack.RichTextElement {
	label := slack.NewRichTextSectionLinkElement(
		row.GetSearchURL(), "🤖 "+row.GetAuthorLabel()+":",
		&slack.RichTextSectionTextStyle{Bold: true},
	)
	numberLinks := utilities.Map(row.PRs, func(pr prview.PR) slack.RichTextSectionElement {
		return slack.NewRichTextSectionLinkElement(
			pr.GetHTMLURL(), fmt.Sprintf("#%d", pr.GetNumber()), &slack.RichTextSectionTextStyle{Bold: true},
		)
	})
	var space slack.RichTextSectionElement = slack.NewRichTextSectionTextElement(" ", &slack.RichTextSectionTextStyle{})
	labelAndNumberLinks := append([]slack.RichTextSectionElement{label}, numberLinks...)
	return slack.NewRichTextSection(utilities.Intersperse(labelAndNumberLinks, space)...)
}

func buildNoOpenPRsBlock(noOpenPRsText string) slack.Block {
	return slack.NewRichTextBlock("no_open_prs",
		slack.NewRichTextSection(
			slack.NewRichTextSectionTextElement(noOpenPRsText, &slack.RichTextSectionTextStyle{}),
		),
	)
}

// A context block, like the footer, so the line reads as a note under the sections. Its text
// holds none of the characters mrkdwn escapes: &, < and >.
func buildMergeStatsLineBlock(stats mergestats.MergeStats, weekBefore *mergestats.MergeStats) slack.Block {
	return slack.NewContextBlock(
		mergeStatsLineBlockID,
		slack.NewTextBlockObject("mrkdwn", mergeStatsLineText(stats, weekBefore), false, false),
	)
}

type shownMedian struct {
	text          string
	percentChange *int
}

func mergeStatsLineText(stats mergestats.MergeStats, weekBefore *mergestats.MergeStats) string {
	mergedCountText := fmt.Sprintf("%s: %d merged", mergeStatsWindowText(stats), stats.MergedCount)
	medians := shownMedians(stats, weekBefore)
	if len(medians) == 0 {
		return mergedCountText
	}
	medianTexts := utilities.Map(medians, func(median shownMedian) string { return median.text })
	lineText := mergedCountText + " · median " + strings.Join(medianTexts, ", ")
	if slices.ContainsFunc(medians, hasNoPercentChange) {
		return lineText
	}
	percentChangeTexts := utilities.Map(medians, func(median shownMedian) string {
		return percentChangeText(*median.percentChange)
	})
	return lineText + " (" + strings.Join(percentChangeTexts, ", ") + ")"
}

func shownMedians(stats mergestats.MergeStats, weekBefore *mergestats.MergeStats) []shownMedian {
	var firstReviewChange, mergeChange *int
	if weekBefore != nil {
		firstReviewChange, mergeChange = mergestats.PercentChanges(stats, *weekBefore)
	}
	var medians []shownMedian
	if stats.MedianTimeToFirstReview != nil {
		medians = append(medians, shownMedian{
			text:          mergeStatsDurationText(*stats.MedianTimeToFirstReview) + " to first review",
			percentChange: firstReviewChange,
		})
	}
	if stats.MedianTimeToMerge != nil {
		medians = append(medians, shownMedian{
			text:          mergeStatsDurationText(*stats.MedianTimeToMerge) + " to merge",
			percentChange: mergeChange,
		})
	}
	return medians
}

func hasNoPercentChange(median shownMedian) bool {
	return median.percentChange == nil
}

func percentChangeText(percentChange int) string {
	if percentChange > 0 {
		return fmt.Sprintf("+%d %%", percentChange)
	}
	return fmt.Sprintf("%d %%", percentChange)
}

// A window under 12 hours rounds to 0 and reads as 1 day. See messagebuilder.spec.md § Oddities
func mergeStatsWindowText(stats mergestats.MergeStats) string {
	windowDays := max(1, int(math.Round(stats.ComputedAt.Sub(stats.MergedSince).Hours()/24)))
	if windowDays == 1 {
		return "Last 1 day"
	}
	return fmt.Sprintf("Last %d days", windowDays)
}

// See messagebuilder.spec.md § Oddities for the rounding at each switch.
func mergeStatsDurationText(duration time.Duration) string {
	switch {
	case duration < time.Hour:
		minutes := int(math.Round(duration.Minutes()))
		if minutes == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", minutes)
	case duration < 24*time.Hour:
		return fmt.Sprintf("%.1f hours", duration.Hours())
	default:
		return fmt.Sprintf("%.1f days", duration.Hours()/24)
	}
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
	maximumContentBlocks := maximumBlocksInSlackMessage - slotsForMergeStatsLine - slotsForFooterOrStalenessWarning
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
