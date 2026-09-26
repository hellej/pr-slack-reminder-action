package githubclient

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

type retryWaitRecorder struct {
	mutex sync.Mutex
	waits []time.Duration
}

func (r *retryWaitRecorder) requestedWaits() []time.Duration {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return slices.Clone(r.waits)
}

func skipAndRecordRetryWaits(t *testing.T) *retryWaitRecorder {
	t.Helper()
	recorder := &retryWaitRecorder{}
	original := waitBeforeRetry
	waitBeforeRetry = func(ctx context.Context, wait time.Duration) error {
		recorder.mutex.Lock()
		defer recorder.mutex.Unlock()
		recorder.waits = append(recorder.waits, wait)
		return ctx.Err()
	}
	t.Cleanup(func() { waitBeforeRetry = original })
	return recorder
}

func withAttemptTimeout(t *testing.T, timeout time.Duration) {
	t.Helper()
	original := attemptTimeout
	attemptTimeout = timeout
	t.Cleanup(func() { attemptTimeout = original })
}

type scriptedAttempts struct {
	results []attemptResult[string]
	calls   int
}

func (s *scriptedAttempts) run(ctx context.Context) attemptResult[string] {
	result := s.results[min(s.calls, len(s.results)-1)]
	s.calls++
	return result
}

func TestRetryTransientFailures(t *testing.T) {
	transientFailure := attemptResult[string]{err: errors.New("bad gateway"), transient: true}
	permanentFailure := attemptResult[string]{err: errors.New("bad credentials")}
	success := attemptResult[string]{value: "fetched"}

	tests := []struct {
		name             string
		results          []attemptResult[string]
		expectedValue    string
		expectedError    string
		expectedAttempts int
		expectedWaits    []time.Duration
		expectedLog      string
	}{
		{
			name:             "success on the first attempt",
			results:          []attemptResult[string]{success},
			expectedValue:    "fetched",
			expectedAttempts: 1,
		},
		{
			name:             "permanent failure is not retried",
			results:          []attemptResult[string]{permanentFailure, success},
			expectedError:    "bad credentials",
			expectedAttempts: 1,
		},
		{
			name:             "transient failure is retried until an attempt succeeds",
			results:          []attemptResult[string]{transientFailure, success},
			expectedValue:    "fetched",
			expectedAttempts: 2,
			expectedWaits:    []time.Duration{2 * time.Second},
			expectedLog:      "artifact list attempt 1 failed, retrying in 2s: bad gateway\n",
		},
		{
			name:             "transient failures stop after the third attempt",
			results:          []attemptResult[string]{transientFailure, transientFailure, transientFailure, success},
			expectedError:    "bad gateway",
			expectedAttempts: 3,
			expectedWaits:    []time.Duration{2 * time.Second, 5 * time.Second},
			expectedLog: "artifact list attempt 1 failed, retrying in 2s: bad gateway\n" +
				"artifact list attempt 2 failed, retrying in 5s: bad gateway\n",
		},
		{
			name:             "permanent failure after a transient one ends the retries",
			results:          []attemptResult[string]{transientFailure, permanentFailure, success},
			expectedError:    "bad credentials",
			expectedAttempts: 2,
			expectedWaits:    []time.Duration{2 * time.Second},
			expectedLog:      "artifact list attempt 1 failed, retrying in 2s: bad gateway\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := skipAndRecordRetryWaits(t)
			logOutput := captureLogOutput(t)
			attempts := &scriptedAttempts{results: tt.results}

			value, err := retryTransientFailures(context.Background(), "artifact list", attempts.run)

			assertEqualStrings(t, "value", value, tt.expectedValue)
			assertEqualStrings(t, "error", errorText(err), tt.expectedError)
			if attempts.calls != tt.expectedAttempts {
				t.Errorf("expected %d attempts, got %d", tt.expectedAttempts, attempts.calls)
			}
			if !slices.Equal(recorder.requestedWaits(), tt.expectedWaits) {
				t.Errorf("expected waits %v, got %v", tt.expectedWaits, recorder.requestedWaits())
			}
			assertEqualStrings(t, "log", logOutput.String(), tt.expectedLog)
		})
	}
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestRetryTransientFailuresStopsWhenTheCallerIsDone(t *testing.T) {
	tests := []struct {
		name             string
		cancelDuringWait bool
	}{
		{name: "caller done before the wait", cancelDuringWait: false},
		{name: "caller done during the wait", cancelDuringWait: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logOutput := captureLogOutput(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			attempts := 0
			started := time.Now()
			_, err := retryTransientFailures(ctx, "GraphQL", func(context.Context) attemptResult[string] {
				attempts++
				if tt.cancelDuringWait {
					time.AfterFunc(20*time.Millisecond, cancel)
				} else {
					cancel()
				}
				return attemptResult[string]{err: errors.New("bad gateway"), transient: true}
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
