package main

import (
	"log"
	"os"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/slackclient"
)

func main() {
	log.SetOutput(os.Stdout)
	// The runner reads a workflow command only at a line's start.
	// See docs/third-party-facts.md § The runner parses workflow commands from stderr as well as stdout, but only at a line's start
	log.SetFlags(0)
	log.Println("Starting PR Slack reminder action")
	err := Run(githubclient.GetAuthenticatedClient, slackclient.GetAuthenticatedClient)
	if err != nil {
		logError(err)
		os.Exit(1)
	}
}
