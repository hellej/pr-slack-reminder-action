package main

import (
	"log"
	"os"

	"github.com/hellej/pr-slack-reminder-action/internal/githubclient"
)

func main() {
	log.Println("Starting PR Slack Reminder...")

	githubToken := os.Getenv("GITHUB_TOKEN")
	// slackWebhook := os.Getenv("SLACK_WEBHOOK_URL")

	if githubToken == "" {
		log.Fatal("Missing required environment variables")
	}

	client := githubclient.GetClient(githubToken)

	log.Printf("GitHub client created successfully %s", client.BaseURL.String())

}
