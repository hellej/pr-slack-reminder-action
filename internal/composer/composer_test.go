package composer_test

import (
	"testing"
	"time"

	"github.com/google/go-github/v72/github"
	"github.com/slack-go/slack"

	composer "github.com/hellej/pr-slack-reminder-action/internal/composer"
)

func TestComposeSlackBlocksMessage(t *testing.T) {
	t.Run("No PRs", func(t *testing.T) {
		prS := []*github.PullRequest{}
		message, _ := composer.ComposeMessage(prS, 24)

		blockLen := len(message.Blocks.BlockSet)
		if blockLen != 1 {
			t.Errorf("Expected there to be exactly one block, got %d", blockLen)
		}

		firstBlock := message.Blocks.BlockSet[0]
		if firstBlock.BlockType() != "rich_text" {
			t.Errorf("Expected first block to be of type 'rich_text', got %s", firstBlock.BlockType())
		}

		richTextElement := firstBlock.(*slack.RichTextBlock).Elements[0].(*slack.RichTextSection).Elements[0].(*slack.RichTextSectionTextElement)
		if richTextElement.Text != "No open PRs in the repository, happy coding! 🎉" {
			t.Errorf("Expected text to be 'No open PRs found', got '%s'", richTextElement.Text)
		}

	})
	t.Run("One new PR", func(t *testing.T) {
		aPR := &github.PullRequest{}
		aPR.CreatedAt = &github.Timestamp{Time: time.Now().Add(-3 * time.Hour)} // 1 day ago
		aPR.Title = github.Ptr("This is a test PR")
		aPR.User = &github.User{
			Login: github.Ptr("testuser"),
			Name:  github.Ptr("Test User"),
		}

		prS := []*github.PullRequest{aPR}
		got, _ := composer.ComposeMessage(prS, 24)

		if len(got.Blocks.BlockSet) < 2 {
			t.Errorf("Expected non-empty blocks, got nil or empty")
		}
		headerBlock := got.Blocks.BlockSet[0].(*slack.HeaderBlock).Text
		if headerBlock.Text != "🚀 New PRs since 1 days ago" {
			t.Errorf("🚀 New PRs since 1 days ago', got '%s'", headerBlock.Text)
		}
		prBulletPointTextElements := got.Msg.Blocks.BlockSet[1].(*slack.RichTextBlock).Elements[0].(*slack.RichTextList).Elements[0].(*slack.RichTextSection).Elements
		prLinkElement := prBulletPointTextElements[0].(*slack.RichTextSectionLinkElement)
		prAfterLinkElement := prBulletPointTextElements[1].(*slack.RichTextSectionTextElement)
		if prLinkElement.Text != "This is a test PR" {
			t.Errorf("Expected text to be 'This is a test PR', got '%s'", prLinkElement.Text)
		}
		if prAfterLinkElement.Text != " (3 hours ago by Test User)" {
			t.Errorf("Expected text to be ' (3 hours ago by Test User)', got '%s'", prAfterLinkElement.Text)
		}
	})
}
