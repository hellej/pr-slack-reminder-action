package main

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/hellej/pr-slack-reminder-action/internal/githubclient"
	composer "github.com/hellej/pr-slack-reminder-action/internal/message-composer"
	"github.com/hellej/pr-slack-reminder-action/internal/slacknotifier"
)

func main() {
	log.Println("Starting PR Slack reminders action...")

	githubToken := os.Getenv("GITHUB_TOKEN")
	repository := os.Getenv("GITHUB_REPOSITORY")
	slackBotToken := os.Getenv("SLACK_BOT_TOKEN")
	slackChannelName := os.Getenv("SLACK_CHANNEL_NAME")

	if githubToken == "" || repository == "" || slackBotToken == "" || slackChannelName == "" {
		log.Fatal("Missing required environment variables")
	}

	githubClient := githubclient.GetClient(githubToken)
	slackClient := slacknotifier.GetClient(slackBotToken)

	repoParts := strings.Split(repository, "/")
	if len(repoParts) != 2 {
		log.Fatalf("Invalid GITHUB_REPOSITORY format: %s", repository)
	}
	repoOwner := repoParts[0]
	repoName := repoParts[1]
	log.Printf("Fetching PRs from repository: %s/%s", repoOwner, repoName)

	prs, _, err := githubClient.PullRequests.List(context.Background(), repoOwner, repoName, nil)
	if err != nil {
		log.Fatalf("Error fetching pull requests: %v", err)
	}
	for _, pr := range prs {
		log.Printf("PR: %s, Title: %s", pr.GetHTMLURL(), pr.GetTitle())
	}

	blocks := composer.ComposeMessage(prs)
	slackErr := slacknotifier.SendMessage(slackClient, slackChannelName, blocks)

	if slackErr != nil {
		log.Fatalf("Error sending message to Slack: %v", slackErr)
	}

}
