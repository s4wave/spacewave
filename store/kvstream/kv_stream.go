package kvstream

import (
	"context"
	"io"
)

// Store is a streaming key/value based store.
type Store interface {
	// Get looks up a key and returns the value.
	// Returns value, if the key was found, and any error.
	Get(
		ctx context.Context,
		key []byte,
	) (val io.ReadCloser, found bool, err error)
	// Set sets a key to a value.
	// If context is canceled, terminate the call.
	Set(
		ctx context.Context,
		key []byte,
		value io.Reader,
	) (err error)
}
