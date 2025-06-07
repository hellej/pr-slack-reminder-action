package main

import (
	"log"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/slackclient"
)

func main() {
	log.Println("Starting PR Slack reminder action")
	err := Run(githubclient.GetAuthenticatedClient, slackclient.GetAuthenticatedClient)
	if err != nil {
		log.Fatalf("%v", err)
	}
}
