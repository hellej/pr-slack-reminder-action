package main_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/go-github/v72/github"
	main "github.com/hellej/pr-slack-reminder-action/cmd/pr-slack-reminder"
	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/slackclient"
	"github.com/hellej/pr-slack-reminder-action/internal/testutils"
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

func makeMockGitHubClientGetter(
	prs []*github.PullRequest,
	responseStatus int,
	err error,
) func(token string) githubclient.Client {
	return func(token string) githubclient.Client {
		return githubclient.NewClient(&mockPullRequestsService{
			mockPRs: prs,
			mockResponse: &github.Response{
				Response: &http.Response{
					StatusCode: responseStatus,
				},
			},
			mockError: err,
		})
	}
}

type SentMessage struct {
	Request   string
	ChannelID string
	Blocks    slack.Message
	Text      string
}

type GetConversationsResponse struct {
	channels []slack.Channel
	cursor   string
	err      error
}

type MockSlackAPI struct {
	getConversationsResponse GetConversationsResponse
	SentMessage              SentMessage
}

func (m *MockSlackAPI) GetConversations(params *slack.GetConversationsParameters) ([]slack.Channel, string, error) {
	if m.getConversationsResponse.err != nil {
		return nil, "", m.getConversationsResponse.err
	}
	return m.getConversationsResponse.channels, m.getConversationsResponse.cursor, nil
}

func (m *MockSlackAPI) PostMessage(
	channelID string, options ...slack.MsgOption,
) (string, string, error) {
	request, values, _ := slack.UnsafeApplyMsgOptions("", "", "", options...)
	m.SentMessage.Request = request
	m.SentMessage.ChannelID = channelID
	m.SentMessage.Text = values["text"][0]
	return "1234567890.123456", "C12345678", nil
}

type slackChannel struct {
	ID   string
	Name string
}

func getMockSlackAPI(slackChannels []slackChannel) *MockSlackAPI {
	if slackChannels == nil {
		slackChannels = []slackChannel{
			{ID: "C12345678", Name: "some-channel-name"},
		}
	}
	channels := make([]slack.Channel, len(slackChannels))
	for i, channel := range slackChannels {
		channels[i] = slack.Channel{
			GroupConversation: slack.GroupConversation{
				Name: channel.Name,
				Conversation: slack.Conversation{
					ID: channel.ID,
				},
			},
		}
	}
	return &MockSlackAPI{
		getConversationsResponse: GetConversationsResponse{
			channels: channels,
			cursor:   "",
			err:      nil,
		},
	}
}

// creates a slackAPI dependency (for injection) if nil is provided
func makeSlackClientGetter(slackAPI *MockSlackAPI) func(token string) slackclient.Client {
	if slackAPI == nil {
		slackAPI = getMockSlackAPI(nil)
	}
	return func(token string) slackclient.Client {
		return slackclient.NewClient(slackAPI)
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
		testutils.AssertPanicStringContains(
			t, recover(), "Either slack-channel-id or slack-channel-name must be set",
		)
	}()
	main.Run(
		makeMockGitHubClientGetter([]*github.PullRequest{}, 200, nil),
		makeSlackClientGetter(nil),
	)
}

func TestWithInvalidRepoInput(t *testing.T) {
	defer func() {
		testutils.AssertPanicStringContains(
			t, recover(), "Invalid GITHUB_REPOSITORY format: invalid/repo/name",
		)
	}()
	setTestEnvironment(t, "invalid/repo/name", "", "")
	main.Run(
		makeMockGitHubClientGetter([]*github.PullRequest{}, 200, nil),
		makeSlackClientGetter(nil),
	)
}

func TestPRFetchError(t *testing.T) {
	defer func() {
		testutils.AssertPanicStringContains(
			t, recover(), "Repository test-org/test-repo not found. Check the repository name and permissions",
		)
	}()
	setTestEnvironment(t, "test-org/test-repo", "", "")
	main.Run(
		makeMockGitHubClientGetter(
			[]*github.PullRequest{},
			404,
			errors.New("Unable to fetch PRs"),
		),
		makeSlackClientGetter(nil),
	)
}

func TestNoPRsFoundWithoutMessage(t *testing.T) {
	setTestEnvironment(t, "", "", "")
	mockSlackAPI := getMockSlackAPI(nil)
	err := main.Run(
		makeMockGitHubClientGetter([]*github.PullRequest{}, 200, nil),
		makeSlackClientGetter(mockSlackAPI),
	)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if mockSlackAPI.SentMessage.Text != "" {
		t.Errorf("Expected no message to be sent, but got: %v", mockSlackAPI.SentMessage.Text)
	}
}

func TestNoPRsFoundWithMessage(t *testing.T) {
	noPRsFoundMessage := "No PRs found, happy coding!"
	setTestEnvironment(t, "", noPRsFoundMessage, "")
	getGitHubClient := makeMockGitHubClientGetter([]*github.PullRequest{}, 200, nil)
	mockSlackAPI := getMockSlackAPI(nil)
	err := main.Run(getGitHubClient, makeSlackClientGetter(mockSlackAPI))
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if mockSlackAPI.SentMessage.Text != noPRsFoundMessage {
		t.Errorf(
			"Expected summary to be %v, but got: %v",
			noPRsFoundMessage,
			mockSlackAPI.SentMessage.Text,
		)
	}
}

func TestWith3PRsFound(t *testing.T) {
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
	getGitHubClient := makeMockGitHubClientGetter(prs, 200, nil)
	mockSlackAPI := getMockSlackAPI(nil)
	err := main.Run(getGitHubClient, makeSlackClientGetter(mockSlackAPI))
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	expectedSummary := "3 open PRs are waiting for attention 👀"
	if mockSlackAPI.SentMessage.Text != expectedSummary {
		t.Errorf(
			"Expected summary to be %v, but got: %v",
			expectedSummary,
			mockSlackAPI.SentMessage.Text,
		)
	}
}
