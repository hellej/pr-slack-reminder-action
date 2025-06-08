package main_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v72/github"
	main "github.com/hellej/pr-slack-reminder-action/cmd/pr-slack-reminder"
	"github.com/hellej/pr-slack-reminder-action/internal/config"
	"github.com/hellej/pr-slack-reminder-action/testhelpers"
	"github.com/hellej/pr-slack-reminder-action/testhelpers/mockgithubclient"
	"github.com/hellej/pr-slack-reminder-action/testhelpers/mockslackclient"
)

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
	c := testhelpers.GetDefaultConfigMinimal()
	testhelpers.SetTestEnvironment(
		t, c,
		&map[string]any{
			config.InputSlackChannelID:   "",
			config.InputSlackChannelName: "",
		},
	)
	err := main.Run(
		mockgithubclient.MakeMockGitHubClientGetter([]*github.PullRequest{}, 200, nil),
		mockslackclient.MakeSlackClientGetter(nil),
	)
	expectedError := "configuration error: either slack-channel-id or slack-channel-name must be set"
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("Expected error: %v, got: %v", expectedError, err)
	}
}

func TestWithOldPRsThresholdButNoHeading(t *testing.T) {
	c := testhelpers.GetDefaultConfigMinimal()
	testhelpers.SetTestEnvironment(
		t, c,
		&map[string]any{
			config.InputOldPRThresholdHours: 10,
			config.InputOldPRsListHeading:   nil,
		},
	)

	err := main.Run(
		mockgithubclient.MakeMockGitHubClientGetter([]*github.PullRequest{}, 200, nil),
		mockslackclient.MakeSlackClientGetter(nil),
	)
	expectedError := "configuration error: if old-pr-threshold-hours is set, old-prs-list-heading must also be set"
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("Expected error: %v, got: %v", expectedError, err)
	}
}

func TestWithInvalidRepoInput(t *testing.T) {
	defer func() {
		testhelpers.AssertPanicStringContains(
			t, recover(), "Invalid GITHUB_REPOSITORY format: invalid/repo/name",
		)
	}()
	setTestEnvironment(t, "invalid/repo/name", "", "")
	main.Run(
		mockgithubclient.MakeMockGitHubClientGetter([]*github.PullRequest{}, 200, nil),
		mockslackclient.MakeSlackClientGetter(nil),
	)
}

func TestPRFetchError(t *testing.T) {
	defer func() {
		testhelpers.AssertPanicStringContains(
			t, recover(), "Repository test-org/test-repo not found. Check the repository name and permissions",
		)
	}()
	setTestEnvironment(t, "test-org/test-repo", "", "")
	main.Run(
		mockgithubclient.MakeMockGitHubClientGetter(
			[]*github.PullRequest{},
			404,
			errors.New("Unable to fetch PRs"),
		),
		mockslackclient.MakeSlackClientGetter(nil),
	)
}

func TestNoPRsFoundWithoutMessage(t *testing.T) {
	setTestEnvironment(t, "", "", "")
	mockSlackAPI := mockslackclient.GetMockSlackAPI(nil, nil, nil)
	err := main.Run(
		mockgithubclient.MakeMockGitHubClientGetter([]*github.PullRequest{}, 200, nil),
		mockslackclient.MakeSlackClientGetter(mockSlackAPI),
	)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if mockSlackAPI.SentMessage.Text != "" {
		t.Errorf("Expected no message to be sent, but got: %v", mockSlackAPI.SentMessage.Text)
	}
}

func TestUnableToGetChannelID(t *testing.T) {
	setTestEnvironment(t, "", "No PRs found, happy coding!", "")
	getGitHubClient := mockgithubclient.MakeMockGitHubClientGetter([]*github.PullRequest{}, 200, nil)
	mockSlackAPI := mockslackclient.GetMockSlackAPI(nil, errors.New("Unable to get channels"), nil)
	err := main.Run(getGitHubClient, mockslackclient.MakeSlackClientGetter(mockSlackAPI))
	expectedError := "error getting channel ID by name: Unable to get channels (check permissions and token)"
	if err.Error() != expectedError {
		t.Errorf("Expected error message '%v', got: %v", expectedError, err.Error())
	}
}

func TestChannelNotFound(t *testing.T) {
	setTestEnvironment(t, "", "No PRs found, happy coding!", "")
	getGitHubClient := mockgithubclient.MakeMockGitHubClientGetter([]*github.PullRequest{}, 200, nil)
	mockSlackChannels := []*mockslackclient.SlackChannel{
		{
			ID:   "C32345678",
			Name: "not-the-channel-name-provided-in-input",
		},
	}
	mockSlackAPI := mockslackclient.GetMockSlackAPI(mockSlackChannels, nil, nil)
	err := main.Run(getGitHubClient, mockslackclient.MakeSlackClientGetter(mockSlackAPI))
	expectedError := "error getting channel ID by name: channel not found"
	if err.Error() != expectedError {
		t.Errorf("Expected error message '%v', got: %v", expectedError, err.Error())
	}
}

func TestNoPRsFoundWithMessage(t *testing.T) {
	noPRsFoundMessage := "No PRs found, happy coding!"
	setTestEnvironment(t, "", noPRsFoundMessage, "")
	getGitHubClient := mockgithubclient.MakeMockGitHubClientGetter([]*github.PullRequest{}, 200, nil)
	mockSlackAPI := mockslackclient.GetMockSlackAPI(nil, nil, nil)
	err := main.Run(getGitHubClient, mockslackclient.MakeSlackClientGetter(mockSlackAPI))
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

func TestWith3PRsFoundWithMinimalConfig(t *testing.T) {
	config := testhelpers.GetDefaultConfigMinimal()
	testhelpers.SetTestEnvironment(t, config, nil)

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
		CreatedAt: &github.Timestamp{Time: time.Now().Add(-26 * time.Hour)},
		Title:     github.Ptr("This is another test PR"),
		User: &github.User{
			Login: github.Ptr("bob"),
		},
	}
	prs := []*github.PullRequest{pr1, pr2, pr3}
	getGitHubClient := mockgithubclient.MakeMockGitHubClientGetter(prs, 200, nil)
	mockSlackAPI := mockslackclient.GetMockSlackAPI(nil, nil, nil)
	err := main.Run(getGitHubClient, mockslackclient.MakeSlackClientGetter(mockSlackAPI))
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

func TestWith3PRsFoundWithFullConfig(t *testing.T) {
	c := testhelpers.GetDefaultConfigFull()
	testhelpers.SetTestEnvironment(t, c, &map[string]any{config.InputOldPRThresholdHours: 12})

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
		CreatedAt: &github.Timestamp{Time: time.Now().Add(-26 * time.Hour)},
		Title:     github.Ptr("This is another test PR"),
		User: &github.User{
			Login: github.Ptr("bob"),
		},
	}
	prs := []*github.PullRequest{pr1, pr2, pr3}
	getGitHubClient := mockgithubclient.MakeMockGitHubClientGetter(prs, 200, nil)
	mockSlackAPI := mockslackclient.GetMockSlackAPI(nil, nil, nil)
	err := main.Run(getGitHubClient, mockslackclient.MakeSlackClientGetter(mockSlackAPI))
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
