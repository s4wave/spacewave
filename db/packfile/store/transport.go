package store

import (
	"context"
	"errors"
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

// transientError marks a transport failure worth one retry: a network error
// or a server-side (5xx) response.
type transientError struct {
	err error
}

// Error returns the wrapped error message.
func (e *transientError) Error() string {
	return e.err.Error()
}

// Unwrap returns the wrapped error.
func (e *transientError) Unwrap() error {
	return e.err
}

// fetchWithRetry runs fetch and retries it once after a transient failure.
func fetchWithRetry(ctx context.Context, fetch func() ([]byte, error)) ([]byte, error) {
	data, err := fetch()
	var transient *transientError
	if err == nil || ctx.Err() != nil || !errors.As(err, &transient) {
		return data, err
	}
	return fetch()
}
