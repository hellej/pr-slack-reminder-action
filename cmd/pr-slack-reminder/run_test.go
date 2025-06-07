package main_test

import (
	"testing"
	"time"

	"github.com/google/go-github/v72/github"
	main "github.com/hellej/pr-slack-reminder-action/cmd/pr-slack-reminder"
	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/slackclient"
	"github.com/slack-go/slack"
)

type mockGitHubClient struct {
	prs []*github.PullRequest
}

func (c mockGitHubClient) FetchOpenPRs(repository string) []*github.PullRequest {
	return c.prs
}

func makeMockGitHubClientGetter(prs []*github.PullRequest) func(token string) githubclient.Client {
	return func(token string) githubclient.Client {
		return mockGitHubClient{prs: prs}
	}
}

type SentMessage struct {
	channelID   string
	blocks      slack.Message
	summaryText string
}

type mockSlackClient struct {
	sentMessage SentMessage
}

func (c mockSlackClient) GetChannelIDByName(channelName string) (string, error) {
	return "C12345678", nil
}

func (c *mockSlackClient) SendMessage(channelID string, blocks slack.Message, summaryText string) error {
	c.sentMessage = SentMessage{
		channelID:   channelID,
		blocks:      blocks,
		summaryText: summaryText,
	}
	return nil
}

func asSlackClientGetter(client *mockSlackClient) func(token string) slackclient.Client {
	return func(token string) slackclient.Client {
		return client
	}
}

func setTestEnvironment(t *testing.T, noPrsMessage string, githubUserSlackIdMapping string) {
	t.Setenv("GITHUB_REPOSITORY", "test-org/test-repo")
	t.Setenv("INPUT_GITHUB-TOKEN", "SOME_TOKEN")
	t.Setenv("INPUT_SLACK-BOT-TOKEN", "SOME_TOKEN")
	t.Setenv("INPUT_SLACK-CHANNEL-NAME", "some-channel-name")
	t.Setenv("INPUT_MAIN-LIST-HEADING", "There are <pr_count> open PRs 🚀")
	if githubUserSlackIdMapping != "" {
		t.Setenv("INPUT_GITHUB-USER-SLACK-USER-ID-MAPPING", githubUserSlackIdMapping)
	}
	if noPrsMessage != "" {
		t.Setenv("INPUT_NO-PRS-MESSAGE", noPrsMessage)
	}
}

func TestNoPRsFoundWithoutMessage(t *testing.T) {
	setTestEnvironment(t, "", "")
	getGitHubClient := makeMockGitHubClientGetter([]*github.PullRequest{})
	mockSlackClient := mockSlackClient{}
	err := main.Run(getGitHubClient, asSlackClientGetter(&mockSlackClient))
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if mockSlackClient.sentMessage.summaryText != "" {
		t.Errorf("Expected no message to be sent, but got: %v", mockSlackClient.sentMessage.summaryText)
	}
}

func TestNoPRsFoundWithMessage(t *testing.T) {
	noPRsFoundMessage := "No PRs found, happy coding!"
	setTestEnvironment(t, noPRsFoundMessage, "")
	getGitHubClient := makeMockGitHubClientGetter([]*github.PullRequest{})
	mockSlackClient := mockSlackClient{}
	err := main.Run(getGitHubClient, asSlackClientGetter(&mockSlackClient))
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if mockSlackClient.sentMessage.summaryText != noPRsFoundMessage {
		t.Errorf(
			"Expected summary to be %v, but got: %v",
			noPRsFoundMessage,
			mockSlackClient.sentMessage.summaryText,
		)
	}
}

func Test2PRsFound(t *testing.T) {
	setTestEnvironment(t, "", "alice: U12345678")
	pr1 := &github.PullRequest{
		CreatedAt: &github.Timestamp{Time: time.Now().Add(-5 * time.Minute)},
		Title:     github.Ptr("This is a test PR"),
		User: &github.User{
			Login: github.Ptr("stitch"),
			Name:  github.Ptr("Stitch"),
		},
	}
	pr2 := &github.PullRequest{
		CreatedAt: &github.Timestamp{Time: time.Now().Add(-3 * time.Hour)},
		Title:     github.Ptr("This is another test PR"),
		User: &github.User{
			Login: github.Ptr("alice"),
			Name:  github.Ptr("Alice"),
		},
	}
	pr3 := &github.PullRequest{
		CreatedAt: &github.Timestamp{Time: time.Now().Add(-25 * time.Hour)},
		Title:     github.Ptr("This is another test PR"),
		User: &github.User{
			Login: github.Ptr("bob"),
		},
	}
	prs := []*github.PullRequest{pr1, pr2, pr3}
	getGitHubClient := makeMockGitHubClientGetter(prs)
	mockSlackClient := mockSlackClient{}
	err := main.Run(getGitHubClient, asSlackClientGetter(&mockSlackClient))
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	expectedSummary := "3 open PRs are waiting for attention 👀"
	if mockSlackClient.sentMessage.summaryText != expectedSummary {
		t.Errorf(
			"Expected summary to be %v, but got: %v",
			expectedSummary,
			mockSlackClient.sentMessage.summaryText,
		)
	}
}
