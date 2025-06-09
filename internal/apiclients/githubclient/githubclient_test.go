package githubclient_test

import (
	"testing"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/githubclient"
)

func TestGetAuthenticatedClient(t *testing.T) {
	client := githubclient.GetAuthenticatedClient("test-token")
	if client == nil {
		t.Fatal("Expected non-nil client, got nil")
	}
}
