package retry_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/retry"
	"github.com/hellej/pr-slack-reminder-action/testhelpers"
	"github.com/hellej/pr-slack-reminder-action/testhelpers/retryhelpers"
)

type scriptedAttempts struct {
	results []retry.AttemptResult[string]
	calls   int
}

func (s *scriptedAttempts) run(ctx context.Context) retry.AttemptResult[string] {
	result := s.results[min(s.calls, len(s.results)-1)]
	s.calls++
	return result
}

func TestTransientFailures(t *testing.T) {
	transientFailure := retry.AttemptResult[string]{Err: errors.New("bad gateway"), Transient: true}
	permanentFailure := retry.AttemptResult[string]{Err: errors.New("bad credentials")}
	success := retry.AttemptResult[string]{Value: "fetched"}

	tests := []struct {
		name             string
		results          []retry.AttemptResult[string]
		expectedValue    string
		expectedError    string
		expectedAttempts int
		expectedWaits    []time.Duration
		expectedLog      string
	}{
		{
			name:             "success on the first attempt",
			results:          []retry.AttemptResult[string]{success},
			expectedValue:    "fetched",
			expectedAttempts: 1,
		},
		{
			name:             "permanent failure is not retried",
			results:          []retry.AttemptResult[string]{permanentFailure, success},
			expectedError:    "bad credentials",
			expectedAttempts: 1,
		},
		{
			name:             "transient failure is retried until an attempt succeeds",
			results:          []retry.AttemptResult[string]{transientFailure, success},
			expectedValue:    "fetched",
			expectedAttempts: 2,
			expectedWaits:    []time.Duration{2 * time.Second},
			expectedLog:      "artifact list attempt 1 failed, retrying in 2s: bad gateway\n",
		},
		{
			name:             "transient failures stop after the third attempt",
			results:          []retry.AttemptResult[string]{transientFailure, transientFailure, transientFailure, success},
			expectedError:    "bad gateway",
			expectedAttempts: 3,
			expectedWaits:    []time.Duration{2 * time.Second, 5 * time.Second},
			expectedLog: "artifact list attempt 1 failed, retrying in 2s: bad gateway\n" +
				"artifact list attempt 2 failed, retrying in 5s: bad gateway\n",
		},
		{
			name:             "permanent failure after a transient one ends the retries",
			results:          []retry.AttemptResult[string]{transientFailure, permanentFailure, success},
			expectedError:    "bad credentials",
			expectedAttempts: 2,
			expectedWaits:    []time.Duration{2 * time.Second},
			expectedLog:      "artifact list attempt 1 failed, retrying in 2s: bad gateway\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retryPolicy, recorder := retryhelpers.SkipAndRecordWaits()
			logOutput := testhelpers.CaptureLog(t)
			attempts := &scriptedAttempts{results: tt.results}

			value, err := retry.TransientFailures(context.Background(), retryPolicy, "artifact list", attempts.run)

			if value != tt.expectedValue {
				t.Errorf("expected value %q, got %q", tt.expectedValue, value)
			}
			if errorText(err) != tt.expectedError {
				t.Errorf("expected error %q, got %q", tt.expectedError, errorText(err))
			}
			if attempts.calls != tt.expectedAttempts {
				t.Errorf("expected %d attempts, got %d", tt.expectedAttempts, attempts.calls)
			}
			if !slices.Equal(recorder.RequestedWaits(), tt.expectedWaits) {
				t.Errorf("expected waits %v, got %v", tt.expectedWaits, recorder.RequestedWaits())
			}
			if logOutput.String() != tt.expectedLog {
				t.Errorf("expected log %q, got %q", tt.expectedLog, logOutput.String())
			}
		})
	}
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestTransientFailuresStopsWhenTheCallerIsDone(t *testing.T) {
	tests := []struct {
		name             string
		cancelDuringWait bool
	}{
		{name: "caller done before the wait", cancelDuringWait: false},
		{name: "caller done during the wait", cancelDuringWait: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logOutput := testhelpers.CaptureLog(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			attempts := 0
			started := time.Now()
			_, err := retry.TransientFailures(ctx, retry.DefaultPolicy(), "GraphQL", func(context.Context) retry.AttemptResult[string] {
				attempts++
				if tt.cancelDuringWait {
					time.AfterFunc(20*time.Millisecond, cancel)
				} else {
					cancel()
				}
				return retry.AttemptResult[string]{Err: errors.New("bad gateway"), Transient: true}
			})

			if attempts != 1 {
				t.Errorf("expected 1 attempt, got %d", attempts)
			}
			if elapsed := time.Since(started); elapsed > time.Second {
				t.Errorf("expected to stop within the 2s wait, took %v", elapsed)
			}
			if !errors.Is(err, context.Canceled) || !strings.Contains(errorText(err), "bad gateway") {
				t.Errorf("expected the attempt's error and the cancellation, got %v", err)
			}
			if tt.cancelDuringWait != strings.Contains(logOutput.String(), "retrying in 2s") {
				t.Errorf("expected a retry log line only when the wait started, got %q", logOutput.String())
			}
		})
	}
}
