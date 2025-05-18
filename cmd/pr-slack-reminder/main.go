package main

import (
	"context"
	"log"
	"os"

	"github.com/hellej/pr-slack-reminder-action/internal/githubclient"
)

func main() {
	log.Println("Starting PR Slack Reminder...")

	githubToken := os.Getenv("GITHUB_TOKEN")
	repo := os.Getenv("GITHUB_REPO")
	repoOwner := os.Getenv("GITHUB_REPO_OWNER")

	// slackWebhook := os.Getenv("SLACK_WEBHOOK_URL")

	if githubToken == "" {
		log.Fatal("Missing required environment variables")
	}

	client := githubclient.GetClient(githubToken)

	log.Printf("GitHub client created successfully %s", client.BaseURL.String())

	prs, _, err := client.PullRequests.List(context.Background(), repoOwner, repo, nil)

	if err != nil {
		log.Fatalf("Error fetching pull requests: %v", err)
	}

	for _, pr := range prs {
		log.Printf("PR: %s, Title: %s", pr.GetHTMLURL(), pr.GetTitle())
	}

}
