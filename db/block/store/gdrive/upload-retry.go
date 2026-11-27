//go:build !tinygo

package block_store_gdrive

import (
	"context"
	"time"
)

// uploadRetry carries the retry deadline to the resumable protocol, which must
// query the committed range before retrying an ambiguously committed chunk.
type uploadRetry struct {
	// cause is sanitized by request before it crosses the transport boundary.
	cause error
	// after is the provider's retry delay or the ordinary exponential backoff.
	after time.Duration
}

// Error returns only the sanitized transport failure.
func (e *uploadRetry) Error() string { return e.cause.Error() }

// Unwrap preserves classification without exposing a provider response body.
func (e *uploadRetry) Unwrap() error { return e.cause }

// waitRetry waits only for a known retry deadline or caller cancellation.
func waitRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-timer.C:
		return nil
	}
}
