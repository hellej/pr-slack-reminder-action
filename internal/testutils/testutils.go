package testutils

import (
	"strings"
	"testing"
)

func AssertIsPanic(t *testing.T, r interface{}) {
	t.Helper()
	if r == nil {
		t.Errorf("Test failed, expected panic but got nil")
	} else {
		t.Log("Panic was caught in test (as expected)!")
	}
}

func AssertPanicStringContains(t *testing.T, r interface{}, expectedSubstring string) {
	t.Helper()
	AssertIsPanic(t, r)
	panicMsg, ok := r.(string)
	if !ok {
		t.Errorf("Expected panic value to be string, got: %T", r)
		return
	}
	if !strings.Contains(panicMsg, expectedSubstring) {
		t.Errorf(
			"Expected panic msg to include ('%v'), got: '%v'",
			expectedSubstring,
			panicMsg,
		)
	}
}
