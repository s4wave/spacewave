package store

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	pkgerrors "github.com/pkg/errors"
)

// Transport fetches raw byte ranges from a remote packfile.
//
// Implementations are the lowest layer in the pack access pipeline: they do
// not cache, deduplicate, or verify anything. The engine wraps a transport in
// a span store (resident bytes), block catalog (block-level publication
// state), and publication queue (verify + writeback).
type Transport interface {
	// Fetch returns bytes [off, off+length) from the pack in a single call.
	// The returned slice has len <= length; a short read signals end-of-pack.
	Fetch(ctx context.Context, off int64, length int) ([]byte, error)
}

// maxRetryWait bounds the delay a rate-limited response can ask for.
const maxRetryWait = time.Minute

// transientError marks a transport failure worth one retry: a network error,
// a server-side (5xx) response, or a rate-limited (429) response.
type transientError struct {
	err error
	// wait is the delay before the retry.
	wait time.Duration
}

// Error returns the wrapped error message.
func (e *transientError) Error() string {
	return e.err.Error()
}

// Unwrap returns the wrapped error.
func (e *transientError) Unwrap() error {
	return e.err
}

// responseError returns the error of an unexpected response status. A 5xx
// response is transient, and a 429 response is transient after the delay its
// Retry-After header names in seconds, up to maxRetryWait.
func responseError(status int, retryAfter string) error {
	err := pkgerrors.Errorf("range request returned status %d", status)
	switch {
	case status >= http.StatusInternalServerError:
		return &transientError{err: err}
	case status == http.StatusTooManyRequests:
		secs, _ := strconv.Atoi(retryAfter)
		return &transientError{err: err, wait: min(time.Duration(max(secs, 1))*time.Second, maxRetryWait)}
	default:
		return err
	}
}

// fetchWithRetry runs fetch and retries it once after a transient failure,
// first waiting the delay the failure names.
func fetchWithRetry(ctx context.Context, fetch func() ([]byte, error)) ([]byte, error) {
	// Fetch, returning anything but a transient failure.
	data, err := fetch()
	var transient *transientError
	if err == nil || ctx.Err() != nil || !errors.As(err, &transient) {
		return data, err
	}

	// Wait out the delay, then retry.
	if transient.wait > 0 {
		timer := time.NewTimer(transient.wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		case <-timer.C:
		}
	}
	return fetch()
}
