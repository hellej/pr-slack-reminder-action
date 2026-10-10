package githubclient

import (
	"slices"
	"testing"
	"time"
)

// A zero policy gives every attempt an already expired deadline, so every request would fail.
func TestGetAuthenticatedClientRetriesUnderTheDefaultPolicy(t *testing.T) {
	retryPolicy := GetAuthenticatedClient("test-token", "").(*client).graphql.retryPolicy

	if !slices.Equal(retryPolicy.WaitsBeforeRetries, []time.Duration{2 * time.Second, 5 * time.Second}) {
		t.Errorf("expected waits [2s 5s], got %v", retryPolicy.WaitsBeforeRetries)
	}
	if retryPolicy.AttemptTimeout != 15*time.Second {
		t.Errorf("expected a 15s attempt deadline, got %v", retryPolicy.AttemptTimeout)
	}
	if retryPolicy.WaitBeforeRetry == nil {
		t.Error("expected a wait before each retry")
	}
}
