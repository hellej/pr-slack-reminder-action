package main

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/hellej/pr-slack-reminder-action/internal/githubclient"
)

func main() {
	log.Println("Starting PR Slack Reminder...")

	githubToken := os.Getenv("GITHUB_TOKEN")
	repository := os.Getenv("GITHUB_REPOSITORY")

	// slackWebhook := os.Getenv("SLACK_WEBHOOK_URL")

	if githubToken == "" || repository == "" {
		log.Fatal("Missing required environment variables")
	}

	client := githubclient.GetClient(githubToken)

	repoParts := strings.Split(repository, "/")

	if len(repoParts) != 2 {
		log.Fatalf("Invalid GITHUB_REPOSITORY format: %s", repository)
	}
	repoOwner := repoParts[0]
	repoName := repoParts[1]

	prs, _, err := client.PullRequests.List(context.Background(), repoOwner, repoName, nil)

	if err != nil {
		log.Fatalf("Error fetching pull requests: %v", err)
	}

	for _, pr := range prs {
		log.Printf("PR: %s, Title: %s", pr.GetHTMLURL(), pr.GetTitle())
	}

}
