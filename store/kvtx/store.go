package kvtx

import (
	"context"
	"time"
)

// Store is a transactional key/value store.
// It can use either read or write transactions.
type Store interface {
	// NewTransaction returns a new transaction against the store.
	// Indicate write if the transaction will not be read-only.
	// Always call Discard() after you are done with the transaction.
	NewTransaction(write bool) (Tx, error)
}

// Tx is a database transaction.
type Tx interface {
	// Get returns values for one or more keys.
	Get(key []byte) (data []byte, found bool, err error)
	// Set sets the value of a key.
	// This will not be committed until Commit is called.
	Set(key, value []byte, ttl time.Duration) error

	// Commit commits the transaction to storage.
	// Can return an error to indicate tx failure.
	// Will return error if called after Discard()
	Commit(ctx context.Context) error
	// Discard cancels the transaction.
	// If called after Commit, does nothing.
	// Cannot return an error.
	// Can be called unlimited times.
	Discard()
}
