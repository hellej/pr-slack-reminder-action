package githubclient

import (
	"context"
	"fmt"
	"log"
	"time"
)

var retryWaits = []time.Duration{2 * time.Second, 5 * time.Second}

// Past GitHub's own 10s processing limit, so its 502 or 504 arrives first. See
// docs/third-party-facts.md § GitHub ends a request after 10 seconds of processing, with a 502 or 504 on GraphQL
var attemptTimeout = 15 * time.Second

// Not a func declaration so that in-package tests can skip and record the waits.
var waitBeforeRetry = func(ctx context.Context, wait time.Duration) error {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type attemptResult[T any] struct {
	value     T
	err       error
	transient bool
}

func retryTransientFailures[T any](
	ctx context.Context, apiName string, runAttempt func(attemptCtx context.Context) attemptResult[T],
) (T, error) {
	result := runAttemptWithDeadline(ctx, runAttempt)
	for retryIndex, wait := range retryWaits {
		if result.err == nil || !result.transient {
			return result.value, result.err
		}
		if ctx.Err() != nil {
			return result.value, fmt.Errorf("%w, not retried: %w", result.err, ctx.Err())
		}
		failedAttemptNumber := retryIndex + 1
		log.Printf("%s attempt %d failed, retrying in %s: %v", apiName, failedAttemptNumber, wait, result.err)
		if err := waitBeforeRetry(ctx, wait); err != nil {
			return result.value, fmt.Errorf("%w, not retried: %w", result.err, err)
		}
		result = runAttemptWithDeadline(ctx, runAttempt)
	}
	return result.value, result.err
}

func runAttemptWithDeadline[T any](
	ctx context.Context, runAttempt func(attemptCtx context.Context) attemptResult[T],
) attemptResult[T] {
	attemptCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
	defer cancel()
	return runAttempt(attemptCtx)
}
