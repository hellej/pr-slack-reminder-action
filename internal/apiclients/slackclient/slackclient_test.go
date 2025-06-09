package slackclient_test

import (
	"testing"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/slackclient"
)

func TestGetAuthenticatedClient(t *testing.T) {
	client := slackclient.GetAuthenticatedClient("test-token")
	if client == nil {
		t.Fatal("Expected non-nil client, got nil")
	}
}
