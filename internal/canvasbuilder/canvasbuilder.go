// Package canvasbuilder renders canvascontent.Content as the markdown of a Slack canvas.
// It names people instead of mentioning them, so refreshing the canvas notifies nobody.
package canvasbuilder

import (
	"fmt"
	"strings"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/canvascontent"
	"github.com/hellej/pr-slack-reminder-action/internal/prview"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

const (
	waitingForReviewHeading  = "## 👀 Waiting for review"
	waitingForAuthorHeading  = "## 💬 Waiting for author"
	readyToMergeHeading      = "## ✅ Ready to merge"
	openPRsHeading           = "## Open"
	mergedPRsHeading         = "## 🚀 Merged"
	wipPRsHeading            = "## 🔧 WIP"
	noOpenPRsText            = "_No open PRs_"
	noWIPPRsText             = "_No work in progress_"
	noMergedPRsText          = "_No merged PRs_"
	mergedPRsUnavailableText = "_Merged PRs could not be fetched_"
)

// The canvas has no top-level heading: Slack renders the canvas title as its own H1 at the top
// of the document, so a body H1 would show as a second title.
func BuildMarkdown(content canvascontent.Content) string {
	blocks := renderOpenSections(content)
	blocks = append(blocks, renderSectionBlocks(section{
		heading:             mergedPRsHeading,
		prSection:           content.Merged,
		groupedByRepository: content.GroupedByRepository,
		renderPRRow:         renderMergedPRRow,
		emptyText:           emptyMergedPRsText(content),
	})...)
	blocks = append(blocks, renderSectionBlocks(section{
		heading:             wipPRsHeading,
		prSection:           content.WIP,
		groupedByRepository: content.GroupedByRepository,
		renderPRRow:         renderWIPPRRow,
		emptyText:           noWIPPRsText,
	})...)
	// A blank block collapses to no space in Slack's canvas renderer: a non-breaking space
	// forces the line to render, giving room above the divider.
	blocks = append(blocks, "\u200B", "---")
	blocks = append(blocks, renderFooter(content)...)
	return strings.Join(blocks, "\n\n") + "\n"
}

// All three buckets empty falls back to the single "Open" heading with "No open PRs" text.
func renderOpenSections(content canvascontent.Content) []string {
	nextActionSections := []section{
		openSection(waitingForReviewHeading, content.WaitingForReview, content.GroupedByRepository),
		openSection(waitingForAuthorHeading, content.WaitingForAuthor, content.GroupedByRepository),
		openSection(readyToMergeHeading, content.ReadyToMerge, content.GroupedByRepository),
	}

	blocks := utilities.FlatMap(utilities.Map(nextActionSections, renderSectionBlocks))
	if len(blocks) > 0 {
		return blocks
	}
	return renderSectionBlocks(section{
		heading:             openPRsHeading,
		groupedByRepository: content.GroupedByRepository,
		renderPRRow:         renderOpenPRRow,
		emptyText:           noOpenPRsText,
	})
}

func openSection(heading string, prSection prview.PRSection, groupedByRepository bool) section {
	return section{
		heading:             heading,
		prSection:           prSection,
		groupedByRepository: groupedByRepository,
		renderPRRow:         renderOpenPRRow,
		hideWhenEmpty:       true,
	}
}

type section struct {
	heading             string
	prSection           prview.PRSection
	groupedByRepository bool
	renderPRRow         func(prview.PR) string
	emptyText           string
	// Drops the heading too, rather than showing it above emptyText.
	hideWhenEmpty bool
}

// Grouped PRs get one sub-heading block per repository, with no repeated section heading.
// Grouping with nothing to show falls back to the same single line the flat section uses.
func renderSectionBlocks(section section) []string {
	// Nil rather than a blank block: strings.Join would keep an empty string as a gap.
	if section.hideWhenEmpty && !section.prSection.HasRows() {
		return nil
	}
	if !section.groupedByRepository || len(section.prSection.Groups) == 0 {
		return []string{renderSection(section.heading, section.prSection.Rows, section)}
	}

	return append(
		[]string{section.heading},
		utilities.Map(section.prSection.Groups, func(group prview.RepositoryRows) string {
			return renderRepositoryGroup(group, section)
		})...,
	)
}

func renderRepositoryGroup(group prview.RepositoryRows, section section) string {
	heading := fmt.Sprintf(
		"### [%s](%s)", escapeMarkdown(group.Repository.Name), group.Repository.GetPullsURL(),
	)
	return renderSection(heading, group.Rows, section)
}

// An empty section shows its empty text under the heading instead of rows.
func renderSection(heading string, rows []prview.Row, section section) string {
	if len(rows) == 0 {
		return heading + "\n\n" + section.emptyText
	}
	renderedRows := utilities.Map(rows, func(row prview.Row) string { return "- " + renderRow(row, section) })
	return heading + "\n\n" + strings.Join(renderedRows, "\n")
}

func renderRow(row prview.Row, section section) string {
	switch row := row.(type) {
	case prview.CollapsedRow:
		return renderCollapsedRow(row)
	default:
		return section.renderPRRow(row.(prview.PR))
	}
}

func renderCollapsedRow(row prview.CollapsedRow) string {
	numberLinks := utilities.Map(row.PRs, func(pr prview.PR) string {
		return fmt.Sprintf("[#%d](%s)", pr.GetNumber(), pr.GetHTMLURL())
	})
	return "🤖 " + escapeMarkdown(row.GetAuthorLabel()) + ": " + strings.Join(numberLinks, " ")
}

func renderOpenPRRow(pr prview.PR) string {
	ageText := "_" + pr.GetPRAgeDisplayText() + "_"
	if pr.IsOldPR {
		ageText = "🚨 `" + pr.GetPRAgeDisplayText() + "`"
	}
	return renderTitleLink(pr) + " " + ageText + renderAuthor(pr) + renderReviewers(pr.Approvers, pr.Commenters)
}

// A WIP PR shows its last activity instead of its age, and never its approvers or the old-PR
// marker. The activity segment is a code span if the PR is active, italics if it's inactive.
func renderWIPPRRow(pr prview.PR) string {
	row := renderTitleLink(pr) + renderAuthor(pr) + renderReviewers(nil, pr.Commenters)

	activityText := pr.GetActivityText()
	if activityText == "" {
		return row
	}
	if pr.IsRecentlyUpdated() {
		return row + " `" + activityText + "`"
	}
	return row + " _" + activityText + "_"
}

func emptyMergedPRsText(content canvascontent.Content) string {
	if content.MergedPRsUnavailable {
		return mergedPRsUnavailableText
	}
	return noMergedPRsText
}

// A merged PR shows when it landed instead of its age.
func renderMergedPRRow(pr prview.PR) string {
	row := renderTitleLink(pr)

	mergedText := pr.GetMergedText()
	if mergedText != "" {
		row += " _" + mergedText + "_"
	}
	return row + renderAuthor(pr) + renderReviewers(pr.Approvers, pr.Commenters)
}

func renderTitleLink(pr prview.PR) string {
	return fmt.Sprintf("**[%s](%s)**", escapeMarkdown(pr.GetTitle()), pr.GetHTMLURL())
}

func renderAuthor(pr prview.PR) string {
	return " by " + escapeMarkdown(pr.Author.GetGitHubName())
}

func renderReviewers(approvers, commenters []prview.Collaborator) string {
	return strings.Join(
		utilities.Map(prview.GetReviewersTextSegments(approvers, commenters), escapeMarkdown),
		"",
	)
}

func renderFooter(content canvascontent.Content) []string {
	updatedText := fmt.Sprintf("_Updated %s UTC_", content.GeneratedAt.UTC().Format("2006-01-02 15:04"))

	capText := getCapText(content)
	if capText == "" {
		return []string{updatedText}
	}
	return []string{capText, updatedText}
}

// Without this note a capped canvas silently misses PRs, and only the run log says why. It names
// the fetch limit, not a row count: `canvascontent` prunes inactive drafts after the fetch, so
// fewer rows than the cap can reach the canvas.
func getCapText(content canvascontent.Content) string {
	switch {
	case content.OpenPRsCapped && content.WIPPRsCapped:
		return fmt.Sprintf(
			"_Fetch limited to the newest %d open PRs and the newest %d WIP PRs_",
			githubclient.MaxPRsToFetch, githubclient.MaxDraftPRsToFetch,
		)
	case content.OpenPRsCapped:
		return fmt.Sprintf("_Fetch limited to the newest %d open PRs_", githubclient.MaxPRsToFetch)
	case content.WIPPRsCapped:
		return fmt.Sprintf("_Fetch limited to the newest %d WIP PRs_", githubclient.MaxDraftPRsToFetch)
	default:
		return ""
	}
}

// A canvas row is one markdown string, so anything coming from GitHub can be read as
// formatting unless it is escaped. Link targets are exempt: they come from GitHub too, but
// can't contain a space or a closing parenthesis.
func escapeMarkdown(text string) string {
	// The backslash goes first, or the later replacements get double-escaped.
	escapable := []string{`\`, "`", "*", "_", "[", "]", "~", "<", ">", "&"}
	for _, character := range escapable {
		text = strings.ReplaceAll(text, character, `\`+character)
	}
	return text
}
