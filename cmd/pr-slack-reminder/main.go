package main

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/hellej/pr-slack-reminder-action/internal/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/slacknotifier"
)

func main() {
	log.Println("Starting PR Slack Reminder...")

	githubToken := os.Getenv("GITHUB_TOKEN")
	repository := os.Getenv("GITHUB_REPOSITORY")
	slackBotToken := os.Getenv("SLACK_BOT_TOKEN")

	if githubToken == "" || repository == "" || slackBotToken == "" {
		log.Fatal("Missing required environment variables")
	}

	githubClient := githubclient.GetClient(githubToken)

	repoParts := strings.Split(repository, "/")

	if len(repoParts) != 2 {
		log.Fatalf("Invalid GITHUB_REPOSITORY format: %s", repository)
	}
	repoOwner := repoParts[0]
	repoName := repoParts[1]

	prs, _, err := githubClient.PullRequests.List(context.Background(), repoOwner, repoName, nil)

	if err != nil {
		log.Fatalf("Error fetching pull requests: %v", err)
	}

	for _, pr := range prs {
		log.Printf("PR: %s, Title: %s", pr.GetHTMLURL(), pr.GetTitle())
	}

	slackClient := slacknotifier.GetClient(slackBotToken)

	slacknotifier.SendMessage(slackClient, "pr-reminders-test", "Hello from PR Slack Reminder!")

}
