package messagebuilder_test

import (
	"testing"
	"time"

	"github.com/slack-go/slack"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/messagebuilder"
	"github.com/hellej/pr-slack-reminder-action/internal/messagecontent"
	"github.com/hellej/pr-slack-reminder-action/internal/prview"
)

var generatedAt = time.Date(2026, 9, 19, 12, 12, 0, 0, time.UTC)

type testPROptions struct {
	title       string
	slackUserID string
	mergedAt    *time.Time
	isOldPR     bool
	approvers   []string
}

func testPR(options testPROptions) prview.PR {
	return prview.PR{
		PR: &githubclient.PR{
			PullRequest: &githubclient.PullRequest{
				Title:     options.title,
				HTMLURL:   "https://github.com/test-org/test-repo/pull/1",
				CreatedAt: time.Now().Add(-3 * time.Hour),
				MergedAt:  options.mergedAt,
				Merged:    options.mergedAt != nil,
			},
		},
		Author: prview.Collaborator{
			Collaborator: &githubclient.Collaborator{Login: "testuser", Name: "Test User"},
			SlackUserID:  options.slackUserID,
		},
		Approvers: approvers(options.approvers),
		IsOldPR:   options.isOldPR,
	}
}

func approvers(names []string) []prview.Collaborator {
	var collaborators []prview.Collaborator
	for _, name := range names {
		collaborators = append(collaborators, prview.Collaborator{
			Collaborator: &githubclient.Collaborator{Login: name, Name: name},
		})
	}
	return collaborators
}

func blockIDs(blocks []slack.Block) []string {
	ids := make([]string, 0, len(blocks))
	for _, block := range blocks {
		switch typedBlock := block.(type) {
		case *slack.RichTextBlock:
			ids = append(ids, typedBlock.BlockID)
		case *slack.HeaderBlock:
			ids = append(ids, typedBlock.BlockID)
		case *slack.SectionBlock:
			ids = append(ids, "spacing")
		case *slack.ContextBlock:
			ids = append(ids, "context")
		default:
			ids = append(ids, "unknown:"+string(block.BlockType()))
		}
	}
	return ids
}

func assertBlockIDs(t *testing.T, message slack.Message, expected []string) {
	t.Helper()
	got := blockIDs(message.Blocks.BlockSet)
	if len(got) != len(expected) {
		t.Fatalf("expected blocks %v, got %v", expected, got)
	}
	for index, id := range got {
		if id != expected[index] {
			t.Fatalf("expected blocks %v, got %v", expected, got)
		}
	}
}

func richTextElements(t *testing.T, block slack.Block) []slack.RichTextElement {
	t.Helper()
	richTextBlock, isRichText := block.(*slack.RichTextBlock)
	if !isRichText {
		t.Fatalf("expected a rich_text block, got %T", block)
	}
	return richTextBlock.Elements
}

func headerBlock(t *testing.T, block slack.Block) *slack.HeaderBlock {
	t.Helper()
	header, isHeader := block.(*slack.HeaderBlock)
	if !isHeader {
		t.Fatalf("expected a header block, got %T", block)
	}
	return header
}

func rowElements(t *testing.T, element slack.RichTextElement, index int) []slack.RichTextSectionElement {
	t.Helper()
	list, isList := element.(*slack.RichTextList)
	if !isList {
		t.Fatalf("expected a rich_text_list, got %T", element)
	}
	return list.Elements[index].(*slack.RichTextSection).Elements
}

func TestEachNonEmptySectionIsAHeaderBlockAndARichTextBlock(t *testing.T) {
	message, summaryText := messagebuilder.BuildMessage(messagecontent.Content{
		SummaryText:      "2 open PRs are waiting for attention 👀",
		WaitingForReview: messagecontent.PRSection{PRs: []prview.PR{testPR(testPROptions{title: "Open PR"})}},
		Merged: messagecontent.PRSection{
			PRs: []prview.PR{testPR(testPROptions{title: "Merged PR", mergedAt: &generatedAt})},
		},
		GeneratedAt: generatedAt,
	})

	assertBlockIDs(t, message, []string{
		"heading_waiting_for_review", "section_waiting_for_review",
		"heading_merged", "section_merged", "context",
	})
	for _, blockIndex := range []int{1, 3} {
		if elements := richTextElements(t, message.Blocks.BlockSet[blockIndex]); len(elements) != 1 {
			t.Errorf("expected one list in an ungrouped section block, got %d elements", len(elements))
		}
	}
	if summaryText != "2 open PRs are waiting for attention 👀" {
		t.Errorf("expected the summary text as the fallback, got %q", summaryText)
	}
}

func TestSectionHeadings(t *testing.T) {
	onePR := messagecontent.PRSection{PRs: []prview.PR{testPR(testPROptions{title: "PR"})}}
	message, _ := messagebuilder.BuildMessage(messagecontent.Content{
		ReadyToMerge:     onePR,
		WaitingForAuthor: onePR,
		WaitingForReview: onePR,
		Merged:           onePR,
		GeneratedAt:      generatedAt,
	})

	assertBlockIDs(t, message, []string{
		"heading_ready_to_merge", "section_ready_to_merge",
		"heading_waiting_for_author", "section_waiting_for_author",
		"heading_waiting_for_review", "section_waiting_for_review",
		"heading_merged", "section_merged", "context",
	})
	expectedHeadings := []string{
		"✅ Ready to merge", "💬 Waiting for author", "👀 Waiting for review", "🚀 Recently merged",
	}
	headingBlocks := []slack.Block{
		message.Blocks.BlockSet[0], message.Blocks.BlockSet[2],
		message.Blocks.BlockSet[4], message.Blocks.BlockSet[6],
	}
	for index, block := range headingBlocks {
		header := headerBlock(t, block)
		if header.Text.Text != expectedHeadings[index] {
			t.Errorf("expected heading %q, got %q", expectedHeadings[index], header.Text.Text)
		}
		if header.Level != 2 {
			t.Errorf("expected heading %q at level 2, got level %d", expectedHeadings[index], header.Level)
		}
		if header.Text.Type != "plain_text" {
			t.Errorf("expected a plain_text heading object, got %q", header.Text.Type)
		}
		if header.Text.Emoji == nil || !*header.Text.Emoji {
			t.Errorf("expected emoji rendering enabled on heading %q", expectedHeadings[index])
		}
	}
}

func assertRepositorySubHeading(t *testing.T, block slack.Block, expectedPath string) {
	t.Helper()
	elements := richTextElements(t, block)
	if len(elements) != 2 {
		t.Fatalf("expected a sub-heading and a list in %q's block, got %d elements", expectedPath, len(elements))
	}
	subHeading, isSection := elements[0].(*slack.RichTextSection)
	if !isSection {
		t.Fatalf("expected a rich_text_section sub-heading, got %T", elements[0])
	}
	if len(subHeading.Elements) != 2 {
		t.Fatalf("expected the path and its colon in the sub-heading, got %d elements", len(subHeading.Elements))
	}
	pathElement, isText := subHeading.Elements[0].(*slack.RichTextSectionTextElement)
	if !isText {
		t.Fatalf("expected an unlinked text sub-heading, got %T", subHeading.Elements[0])
	}
	if pathElement.Style == nil || !pathElement.Style.Bold {
		t.Errorf("expected sub-heading %q bold", expectedPath)
	}
	colonElement, isText := subHeading.Elements[1].(*slack.RichTextSectionTextElement)
	if !isText {
		t.Fatalf("expected a text colon after the sub-heading, got %T", subHeading.Elements[1])
	}
	if colonElement.Style == nil || !colonElement.Style.Bold {
		t.Errorf("expected the colon after %q bold", expectedPath)
	}
	// The path and the colon are separate elements, so only their concatenation shows whether
	// either one carries a stray space.
	subHeadingText := pathElement.Text + colonElement.Text
	if subHeadingText != expectedPath+":" {
		t.Errorf("expected the sub-heading to read %q, got %q", expectedPath+":", subHeadingText)
	}
}

func assertSpacingBlock(t *testing.T, block slack.Block) {
	t.Helper()
	sectionBlock, isSection := block.(*slack.SectionBlock)
	if !isSection {
		t.Fatalf("expected a spacing section block, got %T", block)
	}
	if sectionBlock.Text == nil || sectionBlock.Text.Text != " " {
		t.Errorf("expected a spacing block of one blank space, got %+v", sectionBlock.Text)
	}
}

// A grouped repository block holds its own rows alone, so the title of its first row identifies
// which repository's rows landed in it.
func firstRowTitle(t *testing.T, block slack.Block) string {
	t.Helper()
	elements := richTextElements(t, block)
	if len(elements) != 2 {
		t.Fatalf("expected a sub-heading and a list in the block, got %d elements", len(elements))
	}
	return rowElements(t, elements[1], 0)[0].(*slack.RichTextSectionLinkElement).Text
}

func linkURLs(blocks []slack.Block) []string {
	var urls []string
	for _, block := range blocks {
		richTextBlock, isRichText := block.(*slack.RichTextBlock)
		if !isRichText {
			continue
		}
		for _, element := range richTextBlock.Elements {
			urls = append(urls, linkURLsOfElement(element)...)
		}
	}
	return urls
}

func linkURLsOfElement(element slack.RichTextElement) []string {
	if section, isSection := element.(*slack.RichTextSection); isSection {
		return linkURLsOfSection(section)
	}
	list, isList := element.(*slack.RichTextList)
	if !isList {
		return []string{"unexpected element: " + string(element.RichTextElementType())}
	}
	var urls []string
	for _, row := range list.Elements {
		urls = append(urls, linkURLsOfSection(row.(*slack.RichTextSection))...)
	}
	return urls
}

func linkURLsOfSection(section *slack.RichTextSection) []string {
	var urls []string
	for _, element := range section.Elements {
		if link, isLink := element.(*slack.RichTextSectionLinkElement); isLink {
			urls = append(urls, link.URL)
		}
	}
	return urls
}

func groupedOverTwoRepositories() messagecontent.PRSection {
	return messagecontent.PRSection{
		Groups: []messagecontent.PRsOfRepository{
			{
				RepositoryPath: "owner/repo-one",
				PRs:            []prview.PR{testPR(testPROptions{title: "PR in repo one"})},
			},
			{
				RepositoryPath: "owner/repo-two",
				PRs:            []prview.PR{testPR(testPROptions{title: "PR in repo two"})},
			},
		},
	}
}

func TestGroupedSectionIsARichTextBlockPerRepositoryWithSpacingBetweenThem(t *testing.T) {
	message, _ := messagebuilder.BuildMessage(messagecontent.Content{
		GroupedByRepository: true,
		WaitingForReview:    groupedOverTwoRepositories(),
		GeneratedAt:         generatedAt,
	})

	assertBlockIDs(t, message, []string{
		"heading_waiting_for_review",
		"section_waiting_for_review_repository_1",
		"spacing",
		"section_waiting_for_review_repository_2",
		"context",
	})
	sectionHeading := headerBlock(t, message.Blocks.BlockSet[0])
	if sectionHeading.Text.Text != "👀 Waiting for review" {
		t.Errorf("expected the section heading text, got %q", sectionHeading.Text.Text)
	}
	if sectionHeading.Level != 2 {
		t.Errorf("expected the section heading at level 2, got level %d", sectionHeading.Level)
	}
	assertRepositorySubHeading(t, message.Blocks.BlockSet[1], "owner/repo-one")
	assertSpacingBlock(t, message.Blocks.BlockSet[2])
	assertRepositorySubHeading(t, message.Blocks.BlockSet[3], "owner/repo-two")
	if title := firstRowTitle(t, message.Blocks.BlockSet[1]); title != "PR in repo one" {
		t.Errorf("expected the first repository's row under its own sub-heading, got %q", title)
	}
	if title := firstRowTitle(t, message.Blocks.BlockSet[3]); title != "PR in repo two" {
		t.Errorf("expected the second repository's row under its own sub-heading, got %q", title)
	}
}

func TestGroupedSectionOverOneRepositoryGetsNoSpacingBlock(t *testing.T) {
	message, _ := messagebuilder.BuildMessage(messagecontent.Content{
		GroupedByRepository: true,
		WaitingForReview: messagecontent.PRSection{
			Groups: []messagecontent.PRsOfRepository{{
				RepositoryPath: "owner/repo-one",
				PRs:            []prview.PR{testPR(testPROptions{title: "PR in repo one"})},
			}},
		},
		GeneratedAt: generatedAt,
	})

	assertBlockIDs(t, message, []string{
		"heading_waiting_for_review", "section_waiting_for_review_repository_1", "context",
	})
}

// testPR gives every PR the same URL, so the only link a grouped message can carry is that one.
func TestGroupedSectionRendersNoRepositoryLink(t *testing.T) {
	message, _ := messagebuilder.BuildMessage(messagecontent.Content{
		GroupedByRepository: true,
		WaitingForReview:    groupedOverTwoRepositories(),
		GeneratedAt:         generatedAt,
	})

	for _, url := range linkURLs(message.Blocks.BlockSet) {
		if url != "https://github.com/test-org/test-repo/pull/1" {
			t.Errorf("expected PR links only in a grouped message, got %q", url)
		}
	}
}

func TestTwoGroupedSectionsKeepTheirBlocksInSectionOrder(t *testing.T) {
	message, _ := messagebuilder.BuildMessage(messagecontent.Content{
		GroupedByRepository: true,
		ReadyToMerge: messagecontent.PRSection{
			Groups: []messagecontent.PRsOfRepository{{
				RepositoryPath: "owner/ready-repo",
				PRs:            []prview.PR{testPR(testPROptions{title: "Ready PR"})},
			}},
		},
		WaitingForReview: groupedOverTwoRepositories(),
		GeneratedAt:      generatedAt,
	})

	assertBlockIDs(t, message, []string{
		"heading_ready_to_merge",
		"section_ready_to_merge_repository_1",
		"heading_waiting_for_review",
		"section_waiting_for_review_repository_1",
		"spacing",
		"section_waiting_for_review_repository_2",
		"context",
	})
	assertRepositorySubHeading(t, message.Blocks.BlockSet[1], "owner/ready-repo")
	assertRepositorySubHeading(t, message.Blocks.BlockSet[3], "owner/repo-one")
	assertRepositorySubHeading(t, message.Blocks.BlockSet[5], "owner/repo-two")
	expectedRowTitles := map[int]string{1: "Ready PR", 3: "PR in repo one", 5: "PR in repo two"}
	for blockIndex, expectedTitle := range expectedRowTitles {
		if title := firstRowTitle(t, message.Blocks.BlockSet[blockIndex]); title != expectedTitle {
			t.Errorf("expected row %q in block %d, got %q", expectedTitle, blockIndex, title)
		}
	}
}

func TestOpenPRRow(t *testing.T) {
	message, _ := messagebuilder.BuildMessage(messagecontent.Content{
		WaitingForReview: messagecontent.PRSection{
			PRs: []prview.PR{testPR(testPROptions{title: "Open PR", slackUserID: "U12345678"})},
		},
		GeneratedAt: generatedAt,
	})

	elements := rowElements(t, richTextElements(t, message.Blocks.BlockSet[1])[0], 0)
	if len(elements) != 4 {
		t.Fatalf("expected title, age, ' by ' and author elements, got %d", len(elements))
	}
	if title := elements[0].(*slack.RichTextSectionLinkElement); title.Text != "Open PR" {
		t.Errorf("expected the PR title as the link text, got %q", title.Text)
	} else if title.Style != nil && title.Style.Strike {
		t.Error("expected no strike-through on an open PR title")
	}
	if age := elements[1].(*slack.RichTextSectionTextElement); age.Text != " 3 hours ago" {
		t.Errorf("expected age ' 3 hours ago', got %q", age.Text)
	}
	if user := elements[3].(*slack.RichTextSectionUserElement); user.UserID != "U12345678" {
		t.Errorf("expected the mapped Slack user, got %q", user.UserID)
	}
}

func TestOldPRWarningMarker(t *testing.T) {
	message, _ := messagebuilder.BuildMessage(messagecontent.Content{
		WaitingForReview: messagecontent.PRSection{
			PRs: []prview.PR{testPR(testPROptions{title: "Old PR", isOldPR: true})},
		},
		GeneratedAt: generatedAt,
	})

	elements := rowElements(t, richTextElements(t, message.Blocks.BlockSet[1])[0], 0)
	warning := elements[1].(*slack.RichTextSectionTextElement)
	if warning.Text != " 🚨 " {
		t.Errorf("expected warning marker ' 🚨 ', got %q", warning.Text)
	}
	age := elements[2].(*slack.RichTextSectionTextElement)
	if age.Text != "3 hours old" {
		t.Errorf("expected age text '3 hours old', got %q", age.Text)
	}
	if age.Style == nil || !age.Style.Bold || !age.Style.Code {
		t.Errorf("expected bold+code style on the age text, got %+v", age.Style)
	}
}

func TestAuthorFallsBackToGitHubName(t *testing.T) {
	message, _ := messagebuilder.BuildMessage(messagecontent.Content{
		WaitingForReview: messagecontent.PRSection{
			PRs: []prview.PR{testPR(testPROptions{title: "Open PR"})},
		},
		GeneratedAt: generatedAt,
	})

	elements := rowElements(t, richTextElements(t, message.Blocks.BlockSet[1])[0], 0)
	author, isText := elements[3].(*slack.RichTextSectionTextElement)
	if !isText {
		t.Fatalf("expected a text element for the author, got %T", elements[3])
	}
	if author.Text != "Test User" {
		t.Errorf("expected author name 'Test User', got %q", author.Text)
	}
}

func TestMergedPRRowShowsMergeTimeAndReviewers(t *testing.T) {
	mergedAt := time.Now().Add(-2 * time.Hour)
	message, _ := messagebuilder.BuildMessage(messagecontent.Content{
		Merged: messagecontent.PRSection{
			PRs: []prview.PR{testPR(testPROptions{
				title: "Merged PR", mergedAt: &mergedAt, isOldPR: true, approvers: []string{"Dana Davis"},
			})},
		},
		GeneratedAt: generatedAt,
	})

	elements := rowElements(t, richTextElements(t, message.Blocks.BlockSet[1])[0], 0)
	texts := make([]string, 0, len(elements))
	for _, element := range elements {
		switch typed := element.(type) {
		case *slack.RichTextSectionLinkElement:
			texts = append(texts, typed.Text)
		case *slack.RichTextSectionTextElement:
			texts = append(texts, typed.Text)
		}
	}
	expected := []string{"Merged PR", " merged 2 hours ago", " by ", "Test User", " (✅ ", "Dana Davis", ")"}
	if len(texts) != len(expected) {
		t.Fatalf("expected row segments %v, got %v", expected, texts)
	}
	for index, text := range texts {
		if text != expected[index] {
			t.Fatalf("expected row segments %v, got %v", expected, texts)
		}
	}
	mergeTime := elements[1].(*slack.RichTextSectionTextElement)
	if mergeTime.Style == nil || !mergeTime.Style.Italic {
		t.Errorf("expected the merge time in italics, got %+v", mergeTime.Style)
	}
}

func TestMergedPRRowWithoutAMergeTimeDropsThatSegment(t *testing.T) {
	message, _ := messagebuilder.BuildMessage(messagecontent.Content{
		Merged:      messagecontent.PRSection{PRs: []prview.PR{testPR(testPROptions{title: "Merged PR"})}},
		GeneratedAt: generatedAt,
	})

	elements := rowElements(t, richTextElements(t, message.Blocks.BlockSet[1])[0], 0)
	if len(elements) != 3 {
		t.Fatalf("expected title, ' by ' and author elements, got %d", len(elements))
	}
}

func TestNoOpenPRsTextRendersAboveTheSections(t *testing.T) {
	message, summaryText := messagebuilder.BuildMessage(messagecontent.Content{
		SummaryText:   "Nothing waiting for review 🎉",
		NoOpenPRsText: "All caught up! 🎉",
		Merged:        messagecontent.PRSection{PRs: []prview.PR{testPR(testPROptions{title: "Merged PR"})}},
		GeneratedAt:   generatedAt,
	})

	assertBlockIDs(t, message, []string{
		"no_open_prs", "heading_merged", "section_merged", "context",
	})
	line := richTextElements(t, message.Blocks.BlockSet[0])[0].(*slack.RichTextSection)
	if text := line.Elements[0].(*slack.RichTextSectionTextElement).Text; text != "All caught up! 🎉" {
		t.Errorf("expected the configured no-PRs message, got %q", text)
	}
	if summaryText != "Nothing waiting for review 🎉" {
		t.Errorf("expected the summary text as the fallback, got %q", summaryText)
	}
}

func TestMessageWithNothingToListIsTheNoOpenPRsLineAndTheFooter(t *testing.T) {
	message, _ := messagebuilder.BuildMessage(messagecontent.Content{
		SummaryText:   "Nothing waiting for review 🎉",
		NoOpenPRsText: "All caught up! 🎉",
		GeneratedAt:   generatedAt,
	})

	assertBlockIDs(t, message, []string{"no_open_prs", "context"})
}

func TestFooterNamesTheRunTimestampInTheReadersOwnTimezone(t *testing.T) {
	message, _ := messagebuilder.BuildMessage(messagecontent.Content{
		SummaryText: "Nothing waiting for review 🎉",
		GeneratedAt: generatedAt,
	})

	assertBlockIDs(t, message, []string{"context"})
	footer := message.Blocks.BlockSet[0].(*slack.ContextBlock)
	text := footer.ContextElements.Elements[0].(*slack.TextBlockObject)
	expected := "_Live, updated <!date^1789819920^{time}|12:12 UTC>_"
	if text.Text != expected {
		t.Errorf("expected footer text %q, got %q", expected, text.Text)
	}
	if text.Type != "mrkdwn" {
		t.Errorf("expected an mrkdwn footer element, got %q", text.Type)
	}
}
