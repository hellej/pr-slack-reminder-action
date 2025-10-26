package config

import (
	"fmt"

	"github.com/hellej/pr-slack-reminder-action/internal/config/inputhelpers"
)

type RunMode string

const (
	RunModePost   RunMode = "post"
	RunModeUpdate RunMode = "update"
)

func getRunMode(inputName string) (RunMode, error) {
	return parseRunMode(inputhelpers.GetInputOr(inputName, string(DefaultRunMode)))
}

// parseRunMode validates a raw string as a RunMode.
// It returns an error for unsupported values; defaulting is handled by callers.
func parseRunMode(raw string) (RunMode, error) {
	switch raw {
	case string(RunModePost):
		return RunModePost, nil
	case string(RunModeUpdate):
		return RunModeUpdate, nil
	default:
		return "", fmt.Errorf("invalid run mode: %s (expected '%s' or '%s')", raw, RunModePost, RunModeUpdate)
	}
}
