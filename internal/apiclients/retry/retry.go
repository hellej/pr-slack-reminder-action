// Package retry retries an API call's transient failures on a fixed schedule, each attempt
// under its own deadline. The caller decides which failures are transient.
package retry

import (
	"context"
	"fmt"
	"log"
	"time"
)

type Policy struct {
	WaitsBeforeRetries []time.Duration
	AttemptTimeout     time.Duration
	WaitBeforeRetry    func(ctx context.Context, wait time.Duration) error
}

func DefaultPolicy() Policy {
	return Policy{
		WaitsBeforeRetries: []time.Duration{2 * time.Second, 5 * time.Second},
		// Past GitHub's own 10s processing limit, so its 502 or 504 arrives first. See
		// docs/third-party-facts.md § GitHub ends a request after 10 seconds of processing, with a 502 or 504 on GraphQL
		AttemptTimeout:  15 * time.Second,
		WaitBeforeRetry: waitUnlessCallerIsDone,
	}
}

func waitUnlessCallerIsDone(ctx context.Context, wait time.Duration) error {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type AttemptResult[T any] struct {
	Value     T
	Err       error
	Transient bool
}

func TransientFailures[T any](
	ctx context.Context, policy Policy, apiName string, runAttempt func(attemptCtx context.Context) AttemptResult[T],
) (T, error) {
	result := runAttemptWithDeadline(ctx, policy.AttemptTimeout, runAttempt)
	for retryIndex, wait := range policy.WaitsBeforeRetries {
		if result.Err == nil || !result.Transient {
			return result.Value, result.Err
		}
		if ctx.Err() != nil {
			return result.Value, fmt.Errorf("%w, not retried: %w", result.Err, ctx.Err())
		}
		failedAttemptNumber := retryIndex + 1
		log.Printf("%s attempt %d failed, retrying in %s: %v", apiName, failedAttemptNumber, wait, result.Err)
		if err := policy.WaitBeforeRetry(ctx, wait); err != nil {
			return result.Value, fmt.Errorf("%w, not retried: %w", result.Err, err)
		}
		result = runAttemptWithDeadline(ctx, policy.AttemptTimeout, runAttempt)
	}
	return result.Value, result.Err
}

func runAttemptWithDeadline[T any](
	ctx context.Context, attemptTimeout time.Duration, runAttempt func(attemptCtx context.Context) AttemptResult[T],
) AttemptResult[T] {
	attemptCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
	defer cancel()
	return runAttempt(attemptCtx)
}
