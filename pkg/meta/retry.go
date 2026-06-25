package meta

import (
	"context"
	"time"
)

// RetryDelays is the production backoff schedule: 30s, 2m, 10m.
var RetryDelays = []time.Duration{
	30 * time.Second,
	2 * time.Minute,
	10 * time.Minute,
}

// Retry executes fn with the standard Meta retry policy.
func Retry(ctx context.Context, fn func(context.Context) error) error {
	var lastErr error
	for attempt := 0; attempt <= len(RetryDelays); attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		lastErr = fn(ctx)
		if lastErr == nil {
			return nil
		}
		if IsInvalidToken(lastErr) || IsMissingResource(lastErr) {
			return lastErr
		}
		if !IsRetryable(lastErr) || attempt == len(RetryDelays) {
			return lastErr
		}
		delay := RetryDelays[attempt]
		if IsRateLimited(lastErr) {
			delay = RetryDelays[attempt]
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return lastErr
}
