// Package canvasbuilder renders canvascontent.Content as the markdown of a Slack canvas.
// It names people instead of mentioning them, so refreshing the canvas notifies nobody.
package canvasbuilder

import (
	"fmt"
	"strings"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/canvascontent"
	"github.com/hellej/pr-slack-reminder-action/internal/prparser"
	"github.com/hellej/pr-slack-reminder-action/internal/utilities"
)

const (
	openPRsHeading           = "## Open"
	wipPRsHeading            = "## WIP"
	mergedPRsHeading         = "## Merged"
	noOpenPRsText            = "_No open PRs_"
	noWIPPRsText             = "_No work in progress_"
	noMergedPRsText          = "_No merged PRs_"
	mergedPRsUnavailableText = "_Merged PRs could not be fetched_"
)

// The canvas has no top-level heading: Slack renders the canvas title as its own H1 at the top
// of the document, so a body H1 would show as a second title.
func BuildMarkdown(content canvascontent.Content) string {
	blocks := renderSectionBlocks(section{
		heading:             openPRsHeading,
		prs:                 content.OpenPRs,
		groups:              content.OpenPRsGroupedByRepository,
		groupedByRepository: content.GroupedByRepository,
		renderRow:           renderOpenPRRow,
		emptyText:           noOpenPRsText,
	})
	blocks = append(blocks, renderSectionBlocks(section{
		heading:             wipPRsHeading,
		prs:                 content.WIPPRs,
		groups:              content.WIPPRsGroupedByRepository,
		groupedByRepository: content.GroupedByRepository,
		renderRow:           renderWIPPRRow,
		emptyText:           noWIPPRsText,
	})...)
	blocks = append(blocks, renderSectionBlocks(section{
		heading:             mergedPRsHeading,
		prs:                 content.MergedPRs,
		groups:              content.MergedPRsGroupedByRepository,
		groupedByRepository: content.GroupedByRepository,
		renderRow:           renderMergedPRRow,
		emptyText:           emptyMergedPRsText(content),
	})...)
	// A blank block collapses to no space in Slack's canvas renderer: a non-breaking space
	// forces the line to render, giving room above the divider.
	blocks = append(blocks, "\u200B", "---")
	blocks = append(blocks, renderFooter(content)...)
	return strings.Join(blocks, "\n\n") + "\n"
}

// One canvas section: its PRs as the flat list or as repository buckets, and how to render a row
// of it.
type section struct {
	heading             string
	prs                 []prparser.PR
	groups              []prparser.RepositoryPRs
	groupedByRepository bool
	renderRow           func(prparser.PR) string
	emptyText           string
}

// Grouped PRs get one sub-heading block per repository, with no repeated section heading.
// Grouping with nothing to show falls back to the same single line the flat section uses.
func renderSectionBlocks(section section) []string {
	if !section.groupedByRepository || len(section.groups) == 0 {
		return []string{renderSection(section.heading, section.prs, section.renderRow, section.emptyText)}
	}

	return append(
		[]string{section.heading},
		utilities.Map(section.groups, func(group prparser.RepositoryPRs) string {
			return renderRepositoryGroup(group, section)
		})...,
	)
}

func renderRepositoryGroup(group prparser.RepositoryPRs, section section) string {
	heading := fmt.Sprintf(
		"### [%s](%s)", escapeMarkdown(group.Repository.GetPath()), group.Repository.GetPullsURL(),
	)
	return renderSection(heading, group.PRs, section.renderRow, section.emptyText)
}

// An empty section keeps its heading and shows the given line instead of rows: a missing
// heading would read as a broken render rather than as "nothing here right now".
func renderSection(
	heading string,
	prs []prparser.PR,
	renderRow func(prparser.PR) string,
	emptyText string,
) string {
	if len(prs) == 0 {
		return heading + "\n\n" + emptyText
	}
	rows := utilities.Map(prs, func(pr prparser.PR) string { return "- " + renderRow(pr) })
	return heading + "\n\n" + strings.Join(rows, "\n")
}

func renderOpenPRRow(pr prparser.PR) string {
	ageText := "_" + pr.GetPRAgeDisplayText() + "_"
	if pr.IsOldPR {
		ageText = "🚨 `" + pr.GetPRAgeDisplayText() + "`"
	}
	return renderTitleLink(pr) + " " + ageText + renderAuthor(pr) + renderReviewers(pr.Approvers, pr.Commenters)
}

// A WIP PR shows its last activity instead of its age, and never its approvers or the old-PR
// marker: nobody has been asked to review a draft yet. The activity segment is a code span while
// the draft is moving, italics once it is idle.
func renderWIPPRRow(pr prparser.PR) string {
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

// A failed merged fetch is not an empty week, so the section says which of the two it is.
func emptyMergedPRsText(content canvascontent.Content) string {
	if content.MergedPRsUnavailable {
		return mergedPRsUnavailableText
	}
	return noMergedPRsText
}

// A merged PR shows when it landed instead of its age, and never its reviewers: the section
// answers what landed, not who reviewed it. Unknown merge time drops just that segment.
func renderMergedPRRow(pr prparser.PR) string {
	row := renderTitleLink(pr)

	mergedText := pr.GetMergedText()
	if mergedText != "" {
		row += " _" + mergedText + "_"
	}
	return row + renderAuthor(pr) + " 🚀"
}

func renderTitleLink(pr prparser.PR) string {
	return fmt.Sprintf("**[%s](%s)**", escapeMarkdown(pr.GetTitle()), pr.GetHTMLURL())
}

func renderAuthor(pr prparser.PR) string {
	return " by " + escapeMarkdown(pr.Author.GetGitHubName())
}

func renderReviewers(approvers, commenters []prparser.Collaborator) string {
	return strings.Join(
		utilities.Map(prparser.GetReviewersTextSegments(approvers, commenters), escapeMarkdown),
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
