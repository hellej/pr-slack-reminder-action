package composer_test

import (
	"testing"
	"time"

	"github.com/google/go-github/v72/github"
	"github.com/slack-go/slack"

	composer "github.com/hellej/pr-slack-reminder-action/internal/composer"
	"github.com/hellej/pr-slack-reminder-action/internal/content"
)

func TestComposeSlackBlocksMessage(t *testing.T) {
	t.Run("No PRs", func(t *testing.T) {
		content := content.Content{
			SummaryText: "No open PRs, happy coding! 🎉",
		}

		message, _ := composer.ComposeMessage(content)

		blockLen := len(message.Blocks.BlockSet)
		if blockLen != 1 {
			t.Errorf("Expected there to be exactly one block, got %d", blockLen)
		}

		firstBlock := message.Blocks.BlockSet[0]
		if firstBlock.BlockType() != "rich_text" {
			t.Errorf("Expected first block to be of type 'rich_text', was '%s'", firstBlock.BlockType())
		}

		richTextElement := firstBlock.(*slack.RichTextBlock).Elements[0].(*slack.RichTextSection).Elements[0].(*slack.RichTextSectionTextElement)
		if richTextElement.Text != content.NoPRsText {
			t.Errorf("Expected text to be '%s', got '%s'", content.NoPRsText, richTextElement.Text)
		}
	})

	t.Run("Message summary", func(t *testing.T) {
		aPR := content.PR{PullRequest: &github.PullRequest{}}
		aPR.CreatedAt = &github.Timestamp{Time: time.Now().Add(-3 * time.Hour)} // 1 day ago
		aPR.Title = github.Ptr("This is a test PR")
		aPR.User = &github.User{
			Login: github.Ptr("testuser"),
			Name:  github.Ptr("Test User"),
		}
		prS := []content.PR{aPR}
		content := content.Content{
			SummaryText:     "1 open PRs are waiting for attention 👀",
			MainListHeading: "🚀 New PRs since 1 days ago",
			MainList:        prS,
		}
		_, got := composer.ComposeMessage(content)
		if got != content.SummaryText {
			t.Errorf("Expected summary to be '%s', got '%s'", content.SummaryText, got)
		}
	})

	t.Run("One new PR", func(t *testing.T) {
		aPR := content.PR{PullRequest: &github.PullRequest{}}
		aPR.CreatedAt = &github.Timestamp{Time: time.Now().Add(-3 * time.Hour)} // 3 hours ago
		aPR.Title = github.Ptr("This is a test PR")
		aPR.User = &github.User{
			Login: github.Ptr("testuser"),
			Name:  github.Ptr("Test User"),
		}
		prs := []content.PR{aPR}
		content := content.Content{
			SummaryText:     "1 open PRs are waiting for attention 👀",
			MainListHeading: "🚀 New PRs since 1 days ago",
			MainList:        prs,
		}
		got, _ := composer.ComposeMessage(content)

		if len(got.Blocks.BlockSet) < 2 {
			t.Errorf("Expected non-empty blocks, got nil or empty")
		}
		headerBlock := got.Blocks.BlockSet[0].(*slack.HeaderBlock).Text
		if headerBlock.Text != content.MainListHeading {
			t.Errorf("Expected '%s', got '%s'", content.MainListHeading, headerBlock.Text)
		}
		prBulletPointTextElements := got.Msg.Blocks.BlockSet[1].(*slack.RichTextBlock).Elements[0].(*slack.RichTextList).Elements[0].(*slack.RichTextSection).Elements
		prLinkElement := prBulletPointTextElements[0].(*slack.RichTextSectionLinkElement)
		prAfterLinkElement := prBulletPointTextElements[1].(*slack.RichTextSectionTextElement)
		if prLinkElement.Text != *aPR.Title {
			t.Errorf("Expected text to be '%s', got '%s'", *aPR.Title, prLinkElement.Text)
		}
		expected := " 3 hours ago by Test User"
		if prAfterLinkElement.Text != expected {
			t.Errorf("Expected text to be '%s', got '%s'", expected, prAfterLinkElement.Text)
		}
	})
}
