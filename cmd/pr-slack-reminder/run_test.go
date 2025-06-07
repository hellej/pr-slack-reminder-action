package main_test

import (
	"errors"
	"testing"

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

type mockSlackClient struct{}

func (c mockSlackClient) GetChannelIDByName(channelName string) (string, error) {
	return "C12345678", nil
}

func (c mockSlackClient) SendMessage(channelID string, blocks slack.Message, summaryText string) error {
	return errors.New("mock error: failed to send message")
}

func getSlackClient(token string) slackclient.Client {
	return mockSlackClient{}
}

func TestRun(t *testing.T) {
	t.Setenv("GITHUB_REPOSITORY", "test-org/test-repo")
	t.Setenv("INPUT_GITHUB-TOKEN", "SOME_TOKEN")
	t.Setenv("INPUT_SLACK-BOT-TOKEN", "SOME_TOKEN")
	t.Setenv("INPUT_SLACK-CHANNEL-NAME", "some-channel-name")
	t.Setenv("INPUT_GITHUB-USER-SLACK-USER-ID-MAPPING", "someuser: USOMEID")
	t.Setenv("INPUT_MAIN-LIST-HEADING", "There are <pr_count> open PRs 🚀")

	t.Run("No PRs found", func(t *testing.T) {
		getGitHubClient := makeMockGitHubClientGetter([]*github.PullRequest{})
		err := main.Run(getGitHubClient, getSlackClient)
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})
}
