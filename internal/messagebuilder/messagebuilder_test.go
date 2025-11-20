package messagebuilder_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/google/go-github/v78/github"
	"github.com/slack-go/slack"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/messagebuilder"
	"github.com/hellej/pr-slack-reminder-action/internal/messagecontent"
	"github.com/hellej/pr-slack-reminder-action/internal/prparser"
)

func TestBuildSlackBlocksMessage(t *testing.T) {
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
			SummaryText:   "1 open PRs are waiting for attention 👀",
			PRListHeading: "🚀 New PRs since 1 days ago",
			PRs:           testPRs.PRs,
		}
		_, got := messagebuilder.BuildMessage(content)
		if got != content.SummaryText {
			t.Errorf("Expected summary to be '%s', got '%s'", content.SummaryText, got)
		}
	})

	t.Run("One new PR", func(t *testing.T) {
		testPRs := getTestPRs()

		content := messagecontent.Content{
			SummaryText:   "1 open PRs are waiting for attention 👀",
			PRListHeading: "🚀 New PRs since 1 days ago",
			PRs:           testPRs.PRs,
		}
		got, _ := messagebuilder.BuildMessage(content)

		if len(got.Blocks.BlockSet) < 2 {
			t.Errorf("Expected non-empty blocks, got nil or empty")
		}
		firstBlock := got.Blocks.BlockSet[0]
		header := firstBlock.(*slack.RichTextBlock).Elements[0].(*slack.RichTextSection).Elements[0].(*slack.RichTextSectionTextElement)
		if header.Text != content.PRListHeading {
			t.Errorf("Expected '%s', got '%s'", content.PRListHeading, header.Text)
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
		if prUserElement.UserID != testPRs.PR1.Author.SlackUserID {
			t.Errorf("Expected text to be '%s', got '%s'", testPRs.PR1.Author.SlackUserID, prUserElement.UserID)
		}
	})

	t.Run("Grouped by repository", func(t *testing.T) {
		content := messagecontent.Content{
			SummaryText:         "2 open PRs are waiting for attention 👀",
			GroupedByRepository: true,
			PRsGroupedByRepository: []messagecontent.PRsOfRepository{
				{
					HeadingPrefix:       "Open PRs in ",
					RepositoryLinkLabel: "owner/repo-name",
					RepositoryLink:      "https://github.com/owner/repo-name",
					PRs:                 getTestPRs().PRs,
				},
				{
					HeadingPrefix:       "Open PRs in ",
					RepositoryLinkLabel: "another-org/special-chars_repo",
					RepositoryLink:      "https://github.com/another-org/special-chars_repo",
					PRs:                 getTestPRs().PRs,
				},
			},
		}

		message, summaryText := messagebuilder.BuildMessage(content)

		if summaryText != content.SummaryText {
			t.Errorf("Expected summary to be '%s', got '%s'", content.SummaryText, summaryText)
		}

		if len(message.Blocks.BlockSet) != 5 {
			t.Errorf("Expected 5 blocks, got %d", len(message.Blocks.BlockSet))
		}

		firstHeadingBlock := message.Blocks.BlockSet[0].(*slack.RichTextBlock)

		firstSection := firstHeadingBlock.Elements[0].(*slack.RichTextSection)
		if len(firstSection.Elements) != 3 { // prefix + link + colon
			t.Errorf("Expected 3 elements in first section, got %d", len(firstSection.Elements))
		}

		prefixElement := firstSection.Elements[0].(*slack.RichTextSectionTextElement)
		if prefixElement.Text != "Open PRs in " {
			t.Errorf("Expected prefix 'Open PRs in ', got '%s'", prefixElement.Text)
		}

		linkElement := firstSection.Elements[1].(*slack.RichTextSectionLinkElement)
		if linkElement.Text != "owner/repo-name" {
			t.Errorf("Expected link text 'owner/repo-name', got '%s'", linkElement.Text)
		}
		if linkElement.URL != "https://github.com/owner/repo-name" {
			t.Errorf("Expected link URL 'https://github.com/owner/repo-name', got '%s'", linkElement.URL)
		}
	})
}

func TestLimitMessageSizeByMaxBlocks(t *testing.T) {
	testCases := []struct {
		name            string
		numRepositories int
		expectedBlocks  int
	}{
		{
			name:            "Within block limit",
			numRepositories: 5,
			expectedBlocks:  14, // 3 blocks per PR (except the last one has only 2)
		},
		{
			name:            "Exceeding block limit",
			numRepositories: 20, // -> 59 blocks
			expectedBlocks:  50,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			repoLists := []messagecontent.PRsOfRepository{}
			repoId := 1
			for repoId <= tc.numRepositories {
				repoLists = append(repoLists, newRepositoryList(repoId))
				repoId += 1
			}

			message, _ := messagebuilder.BuildMessage(
				messagecontent.Content{
					SummaryText:            "2 open PRs are waiting for attention 👀",
					GroupedByRepository:    true,
					PRsGroupedByRepository: repoLists,
				},
			)

			if len(message.Blocks.BlockSet) != tc.expectedBlocks {
				t.Errorf("Expected %d blocks, got %d", tc.expectedBlocks, len(message.Blocks.BlockSet))
			}
		})
	}
}

func newRepositoryList(id int) messagecontent.PRsOfRepository {
	return messagecontent.PRsOfRepository{
		HeadingPrefix:       "Open PRs in repo " + strconv.Itoa(id),
		RepositoryLinkLabel: "owner/repo-" + strconv.Itoa(id),
		RepositoryLink:      "https://github.com/owner/repo-" + strconv.Itoa(id),
		PRs:                 []prparser.PR{getTestPRs().PR1},
	}
}

type TestPRs struct {
	PR1 prparser.PR
	PRs []prparser.PR
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
		Author: prparser.Collaborator{
			Collaborator: &githubclient.Collaborator{
				Login: "Test User",
			},
			SlackUserID: "U12345678",
		},
	}
	return TestPRs{
		PR1: pr1,
		PRs: []prparser.PR{pr1},
	}
}

func TestMergedAndClosedPRFormatting(t *testing.T) {
	testCases := []struct {
		name                    string
		pr                      prparser.PR
		expectedStrikethrough   bool
		expectedMergedIndicator bool
		expectedReviewerSection bool
	}{
		{
			name: "Open PR - no special formatting",
			pr: prparser.PR{
				PR: &githubclient.PR{
					PullRequest: &github.PullRequest{
						CreatedAt: &github.Timestamp{Time: time.Now().Add(-3 * time.Hour)},
						Title:     github.Ptr("Open PR"),
						State:     github.Ptr("open"),
						Merged:    github.Ptr(false),
						User: &github.User{
							Login: github.Ptr("alice"),
							Name:  github.Ptr("Alice"),
						},
					},
				},
				Author: prparser.Collaborator{
					Collaborator: &githubclient.Collaborator{Login: "alice", Name: "Alice"},
				},
			},
			expectedStrikethrough:   false,
			expectedMergedIndicator: false,
			expectedReviewerSection: false,
		},
		{
			name: "Merged PR with reviewers",
			pr: prparser.PR{
				PR: &githubclient.PR{
					PullRequest: &github.PullRequest{
						CreatedAt: &github.Timestamp{Time: time.Now().Add(-3 * time.Hour)},
						Title:     github.Ptr("Merged PR"),
						State:     github.Ptr("closed"),
						Merged:    github.Ptr(true),
						User: &github.User{
							Login: github.Ptr("bob"),
							Name:  github.Ptr("Bob"),
						},
					},
				},
				Author: prparser.Collaborator{
					Collaborator: &githubclient.Collaborator{Login: "bob", Name: "Bob"},
				},
				Approvers: []prparser.Collaborator{
					{Collaborator: &githubclient.Collaborator{Login: "reviewer1", Name: "Reviewer One"}},
				},
			},
			expectedStrikethrough:   false, // Merged PRs should NOT have strikethrough
			expectedMergedIndicator: true,
			expectedReviewerSection: true,
		},
		{
			name: "Closed PR without merge",
			pr: prparser.PR{
				PR: &githubclient.PR{
					PullRequest: &github.PullRequest{
						CreatedAt: &github.Timestamp{Time: time.Now().Add(-3 * time.Hour)},
						Title:     github.Ptr("Closed PR"),
						State:     github.Ptr("closed"),
						Merged:    github.Ptr(false),
						User: &github.User{
							Login: github.Ptr("charlie"),
							Name:  github.Ptr("Charlie"),
						},
					},
				},
				Author: prparser.Collaborator{
					Collaborator: &githubclient.Collaborator{Login: "charlie", Name: "Charlie"},
				},
			},
			expectedStrikethrough:   true,
			expectedMergedIndicator: false,
			expectedReviewerSection: false,
		},
		{
			name: "Merged PR without reviewers",
			pr: prparser.PR{
				PR: &githubclient.PR{
					PullRequest: &github.PullRequest{
						CreatedAt: &github.Timestamp{Time: time.Now().Add(-3 * time.Hour)},
						Title:     github.Ptr("Merged PR no reviewers"),
						State:     github.Ptr("closed"),
						Merged:    github.Ptr(true),
						User: &github.User{
							Login: github.Ptr("dave"),
							Name:  github.Ptr("Dave"),
						},
					},
				},
				Author: prparser.Collaborator{
					Collaborator: &githubclient.Collaborator{Login: "dave", Name: "Dave"},
				},
			},
			expectedStrikethrough:   false, // Merged PRs should NOT have strikethrough
			expectedMergedIndicator: true,
			expectedReviewerSection: false, // No reviewers, so no reviewer section
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			content := messagecontent.Content{
				SummaryText:   "Test",
				PRListHeading: "Test PRs",
				PRs:           []prparser.PR{tc.pr},
			}

			message, _ := messagebuilder.BuildMessage(content)

			prBlock := message.Blocks.BlockSet[1].(*slack.RichTextBlock)
			prSection := prBlock.Elements[0].(*slack.RichTextList).Elements[0].(*slack.RichTextSection)

			linkElement := prSection.Elements[0].(*slack.RichTextSectionLinkElement)

			if tc.expectedStrikethrough {
				if linkElement.Style == nil || !linkElement.Style.Strike {
					t.Error("Expected strikethrough formatting on PR title but it was not applied")
				}
			} else {
				if linkElement.Style != nil && linkElement.Style.Strike {
					t.Error("Did not expect strikethrough formatting on PR title but it was applied")
				}
			}

			hasReviewerSection := false
			hasMergedIndicator := false

			for _, element := range prSection.Elements {
				if textElement, ok := element.(*slack.RichTextSectionTextElement); ok {
					if textElement.Text == " 🔀" {
						hasMergedIndicator = true
					}
					if textElement.Text == " (✅ " || textElement.Text == " (💬 " || textElement.Text == " (" {
						hasReviewerSection = true
					}
				}
			}

			if tc.expectedReviewerSection && !hasReviewerSection {
				t.Error("Expected reviewer section but it was not found")
			}
			if !tc.expectedReviewerSection && hasReviewerSection {
				t.Error("Did not expect reviewer section but it was found")
			}
			if tc.expectedMergedIndicator && !hasMergedIndicator {
				t.Error("Expected merged indicator (🔀) but it was not found")
			}
			if !tc.expectedMergedIndicator && hasMergedIndicator {
				t.Error("Did not expect merged indicator (🔀) but it was found")
			}
		})
	}
}
