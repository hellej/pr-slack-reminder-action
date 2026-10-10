package retryhelpers

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/hellej/pr-slack-reminder-action/internal/apiclients/retry"
)

type WaitRecorder struct {
	mutex sync.Mutex
	waits []time.Duration
}

func (r *WaitRecorder) RequestedWaits() []time.Duration {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return slices.Clone(r.waits)
}

// SkipAndRecordWaits returns retry.DefaultPolicy with each wait recorded instead of slept.
func SkipAndRecordWaits() (retry.Policy, *WaitRecorder) {
	recorder := &WaitRecorder{}
	policy := retry.DefaultPolicy()
	policy.WaitBeforeRetry = func(ctx context.Context, wait time.Duration) error {
		recorder.mutex.Lock()
		defer recorder.mutex.Unlock()
		recorder.waits = append(recorder.waits, wait)
		return ctx.Err()
	}
	return policy, recorder
}
