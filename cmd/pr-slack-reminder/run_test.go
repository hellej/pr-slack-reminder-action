package main_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v72/github"
	main "github.com/hellej/pr-slack-reminder-action/cmd/pr-slack-reminder"
	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/slackclient"
	"github.com/slack-go/slack"
)

type mockPullRequestsService struct {
	mockPRs      []*github.PullRequest
	mockResponse *github.Response
	mockError    error
}

func (m *mockPullRequestsService) List(
	ctx context.Context, owner string, repo string, opts *github.PullRequestListOptions,
) ([]*github.PullRequest, *github.Response, error) {
	return m.mockPRs, m.mockResponse, m.mockError
}

func makeMockGitHubClientGetter(prs []*github.PullRequest) func(token string) githubclient.Client {
	return func(token string) githubclient.Client {
		return githubclient.NewClient(&mockPullRequestsService{
			mockPRs: prs,
			mockResponse: &github.Response{
				Response: &http.Response{
					StatusCode: 200,
				},
			},
			mockError: nil,
		})
	}
}

func makeMockGitHubClientGetterWithPRService(prService mockPullRequestsService) func(token string) githubclient.Client {
	return func(token string) githubclient.Client {
		return githubclient.NewClient(&prService)
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

func setTestEnvironment(t *testing.T, repository string, noPrsMessage string, githubUserSlackIdMapping string) {
	if repository != "" {
		t.Setenv("GITHUB_REPOSITORY", repository)
	} else {
		t.Setenv("GITHUB_REPOSITORY", "test-org/test-repo")
	}
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

func TestWithMissingSlackInputs(t *testing.T) {
	t.Setenv("GITHUB_REPOSITORY", "test-org/test-repo")
	t.Setenv("INPUT_GITHUB-TOKEN", "SOME_TOKEN")
	t.Setenv("INPUT_SLACK-BOT-TOKEN", "SOME_TOKEN")
	t.Setenv("INPUT_MAIN-LIST-HEADING", "There are <pr_count> open PRs 🚀")

	defer func() {
		if r := recover(); r != nil {
			err, ok := r.(string)
			if !ok {
				t.Errorf("Expected panic value to be string, got: %T", r)
				return
			}
			if !strings.Contains(err, "Either slack-channel-id or slack-channel-name must be set") {
				t.Errorf("Test failed, expected panic with specific message, got: %v", err)
			}
		}
	}()
	main.Run(
		makeMockGitHubClientGetter([]*github.PullRequest{}),
		asSlackClientGetter(&mockSlackClient{}),
	)
	t.Errorf("Test failed, panic was expected")
}

func TestWithInvalidRepoInput(t *testing.T) {
	setTestEnvironment(t, "invalid/repo/name", "", "")

	defer func() {
		if r := recover(); r != nil {
			err, ok := r.(string)
			if !ok {
				t.Errorf("Expected panic value to be string, got: %T", r)
				return
			}
			if !strings.Contains(err, "Invalid GITHUB_REPOSITORY format: invalid/repo/name") {
				t.Errorf("Test failed, expected panic with specific message, got: %v", err)
			}
		}
	}()
	main.Run(
		makeMockGitHubClientGetter([]*github.PullRequest{}),
		asSlackClientGetter(&mockSlackClient{}),
	)
	t.Errorf("Test failed, panic was expected")
}

func TestPRFetchError(t *testing.T) {
	setTestEnvironment(t, "", "", "")
	defer func() {
		if r := recover(); r != nil {
			err, ok := r.(string)
			if !ok {
				t.Errorf("Expected panic value to be string, got: %T", r)
				return
			}
			if !strings.Contains(err, "Repository test-org/test-repo not found") {
				t.Errorf("Test failed, expected panic with specific message, got: %v", err)
			}
		}
	}()
	main.Run(
		makeMockGitHubClientGetterWithPRService(
			mockPullRequestsService{
				mockPRs: []*github.PullRequest{},
				mockResponse: &github.Response{
					Response: &http.Response{
						StatusCode: 404,
					},
				},
				mockError: errors.New("Unable to fetch PRs"),
			},
		),
		asSlackClientGetter(&mockSlackClient{}),
	)
	t.Errorf("Test failed, panic was expected")
}

func TestNoPRsFoundWithoutMessage(t *testing.T) {
	setTestEnvironment(t, "", "", "")
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
	setTestEnvironment(t, "", noPRsFoundMessage, "")
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

func Test3PRsFound(t *testing.T) {
	setTestEnvironment(t, "", "", "alice: U12345678")
	pr1 := &github.PullRequest{
		Number:    github.Ptr(1),
		CreatedAt: &github.Timestamp{Time: time.Now().Add(-5 * time.Minute)},
		Title:     github.Ptr("This is a test PR"),
		User: &github.User{
			Login: github.Ptr("stitch"),
			Name:  github.Ptr("Stitch"),
		},
	}
	pr2 := &github.PullRequest{
		Number:    github.Ptr(2),
		CreatedAt: &github.Timestamp{Time: time.Now().Add(-3 * time.Hour)},
		Title:     github.Ptr("This is another test PR"),
		User: &github.User{
			Login: github.Ptr("alice"),
			Name:  github.Ptr("Alice"),
		},
	}
	pr3 := &github.PullRequest{
		Number:    github.Ptr(3),
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
