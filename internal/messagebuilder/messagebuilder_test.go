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

func sectionElements(t *testing.T, block slack.Block) []slack.RichTextElement {
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

func TestGroupedSectionHoldsEveryRepositoryInOneBlock(t *testing.T) {
	message, _ := messagebuilder.BuildMessage(messagecontent.Content{
		GroupedByRepository: true,
		WaitingForReview: messagecontent.PRSection{
			Groups: []messagecontent.PRsOfRepository{
				{
					RepositoryLinkLabel: "owner/repo-one",
					RepositoryLink:      "https://github.com/owner/repo-one/pulls",
					PRs:                 []prview.PR{testPR(testPROptions{title: "PR in repo one"})},
				},
				{
					RepositoryLinkLabel: "owner/repo-two",
					RepositoryLink:      "https://github.com/owner/repo-two/pulls",
					PRs:                 []prview.PR{testPR(testPROptions{title: "PR in repo two"})},
				},
			},
		},
		GeneratedAt: generatedAt,
	})

	assertBlockIDs(t, message, []string{
		"heading_waiting_for_review", "section_waiting_for_review", "context",
	})
	elements := sectionElements(t, message.Blocks.BlockSet[1])
	if len(elements) != 4 {
		t.Fatalf("expected two sub-headings and two lists, got %d elements", len(elements))
	}

	subHeading := elements[0].(*slack.RichTextSection)
	linkElement := subHeading.Elements[0].(*slack.RichTextSectionLinkElement)
	if linkElement.Text != "owner/repo-one" {
		t.Errorf("expected sub-heading link text 'owner/repo-one', got %q", linkElement.Text)
	}
	if linkElement.URL != "https://github.com/owner/repo-one/pulls" {
		t.Errorf("expected the repository pulls URL, got %q", linkElement.URL)
	}
	if linkElement.Style == nil || !linkElement.Style.Bold {
		t.Error("expected the repository sub-heading link to be bold")
	}
	colonElement := subHeading.Elements[1].(*slack.RichTextSectionTextElement)
	if colonElement.Text != ":" {
		t.Errorf("expected ':' after the repository link, got %q", colonElement.Text)
	}
	if colonElement.Style == nil || !colonElement.Style.Bold {
		t.Error("expected the ':' after the repository link to be bold")
	}
}

func TestOpenPRRow(t *testing.T) {
	message, _ := messagebuilder.BuildMessage(messagecontent.Content{
		WaitingForReview: messagecontent.PRSection{
			PRs: []prview.PR{testPR(testPROptions{title: "Open PR", slackUserID: "U12345678"})},
		},
		GeneratedAt: generatedAt,
	})

	elements := rowElements(t, sectionElements(t, message.Blocks.BlockSet[1])[0], 0)
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

	elements := rowElements(t, sectionElements(t, message.Blocks.BlockSet[1])[0], 0)
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

	elements := rowElements(t, sectionElements(t, message.Blocks.BlockSet[1])[0], 0)
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

	elements := rowElements(t, sectionElements(t, message.Blocks.BlockSet[1])[0], 0)
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

	elements := rowElements(t, sectionElements(t, message.Blocks.BlockSet[1])[0], 0)
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
	line := sectionElements(t, message.Blocks.BlockSet[0])[0].(*slack.RichTextSection)
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
