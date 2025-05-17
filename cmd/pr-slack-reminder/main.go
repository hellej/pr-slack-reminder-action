package main

import (
	"log"
	"os"
)

func main() {
	log.Println("Starting PR Slack Reminder...")

	githubToken := os.Getenv("GITHUB_TOKEN")
	slackWebhook := os.Getenv("SLACK_WEBHOOK_URL")

	if githubToken == "" || slackWebhook == "" {
		log.Fatal("Missing required environment variables")
	}
}
