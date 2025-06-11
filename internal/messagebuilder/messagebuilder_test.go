package messagebuilder_test

import (
	"testing"
	"time"

	"github.com/google/go-github/v72/github"
	"github.com/slack-go/slack"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/messagebuilder"
	"github.com/hellej/pr-slack-reminder-action/internal/messagecontent"
	"github.com/hellej/pr-slack-reminder-action/internal/prparser"
)

func TestComposeSlackBlocksMessage(t *testing.T) {
	t.Run("No PRs", func(t *testing.T) {
		content := messagecontent.Content{
			SummaryText: "No open PRs, happy coding! 🎉",
		}

		message, _ := messagebuilder.BuildMessage(content)

		blockLen := len(message.Blocks.BlockSet)
		if blockLen != 1 {
			t.Errorf("Expected there to be exactly one block, got %d", blockLen)
		}

		firstBlock := message.Blocks.BlockSet[0]
		if firstBlock.BlockType() != "rich_text" {
			t.Errorf("Expected first block to be of type 'rich_text', was '%s'", firstBlock.BlockType())
		}

		richTextElement := firstBlock.(*slack.RichTextBlock).Elements[0].(*slack.RichTextSection).Elements[0].(*slack.RichTextSectionTextElement)
		if richTextElement.Text != content.SummaryText {
			t.Errorf("Expected text to be '%s', got '%s'", content.SummaryText, richTextElement.Text)
		}
	})

	t.Run("Message summary", func(t *testing.T) {
		testPRs := getTestPRs()
		content := messagecontent.Content{
			SummaryText:     "1 open PRs are waiting for attention 👀",
			MainListHeading: "🚀 New PRs since 1 days ago",
			MainList:        testPRs.PRs,
		}
		_, got := messagebuilder.BuildMessage(content)
		if got != content.SummaryText {
			t.Errorf("Expected summary to be '%s', got '%s'", content.SummaryText, got)
		}
	})

	t.Run("One new PR", func(t *testing.T) {
		testPRs := getTestPRs()

		content := messagecontent.Content{
			SummaryText:     "1 open PRs are waiting for attention 👀",
			MainListHeading: "🚀 New PRs since 1 days ago",
			MainList:        testPRs.PRs,
		}
		got, _ := messagebuilder.BuildMessage(content)

		if len(got.Blocks.BlockSet) < 2 {
			t.Errorf("Expected non-empty blocks, got nil or empty")
		}
		headerBlock := got.Blocks.BlockSet[0].(*slack.HeaderBlock).Text
		if headerBlock.Text != content.MainListHeading {
			t.Errorf("Expected '%s', got '%s'", content.MainListHeading, headerBlock.Text)
		}
		prBulletPointTextElements := got.Msg.Blocks.BlockSet[1].(*slack.RichTextBlock).Elements[0].(*slack.RichTextList).Elements[0].(*slack.RichTextSection).Elements
		prLinkElement := prBulletPointTextElements[0].(*slack.RichTextSectionLinkElement)
		prAgeElement := prBulletPointTextElements[1].(*slack.RichTextSectionTextElement)
		prBeforeUserElement := prBulletPointTextElements[2].(*slack.RichTextSectionTextElement)
		prUserElement := prBulletPointTextElements[3].(*slack.RichTextSectionUserElement)
		if prLinkElement.Text != *testPRs.PR1.Title {
			t.Errorf("Expected text to be '%s', got '%s'", *testPRs.PR1.Title, prLinkElement.Text)
		}
		expectedAgeText := " 3 hours ago"
		if prAgeElement.Text != expectedAgeText {
			t.Errorf("Expected text to be '%s', got '%s'", expectedAgeText, prAgeElement.Text)
		}
		expectedBeforeUserText := " by "
		if prBeforeUserElement.Text != expectedBeforeUserText {
			t.Errorf("Expected text to be '%s', got '%s'", expectedBeforeUserText, prAgeElement.Text)
		}
		if prUserElement.UserID != testPRs.PR1.AuthorInfo.SlackUserID {
			t.Errorf("Expected text to be '%s', got '%s'", testPRs.PR1.AuthorInfo.SlackUserID, prUserElement.UserID)
		}
	})
}

type TestPRs struct {
	PRs []prparser.PR
	PR1 prparser.PR
}

func getTestPRs() TestPRs {
	pr1 := prparser.PR{
		PR: &githubclient.PR{
			PullRequest: &github.PullRequest{
				CreatedAt: &github.Timestamp{Time: time.Now().Add(-3 * time.Hour)}, // 1 day ago
				Title:     github.Ptr("This is a test PR"),
				User: &github.User{
					Login: github.Ptr("testuser"),
					Name:  github.Ptr("Test User"),
				},
			},
		},
	}
	pr1.AuthorInfo = prparser.Collaborator{
		GitHubName:  "Test User",
		SlackUserID: "U12345678",
	}
	return TestPRs{
		PRs: []prparser.PR{pr1},
		PR1: pr1,
	}
}
