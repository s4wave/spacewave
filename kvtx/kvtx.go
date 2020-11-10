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

// TxOps contains the database transaction operations.
type TxOps interface {
	// Get returns values for a key.
	Get(key []byte) (data []byte, found bool, err error)
	// Set sets the value of a key.
	// This will not be committed until Commit is called.
	Set(key, value []byte, ttl time.Duration) error
	// Delete deletes a key.
	// This will not be committed until Commit is called.
	// Not found should not return an error.
	Delete(key []byte) error
	// ScanPrefix iterates over keys with a prefix.
	//
	// Note: neither key nor value should be retained outside cb() without
	// copying.
	//
	// Note: the ordering of the scan is not necessarily sorted.
	ScanPrefix(prefix []byte, cb func(key, value []byte) error) error
	// Exists checks if a key exists.
	Exists(key []byte) (bool, error)
}

// Tx is a database transaction.
// Concurrent calls are not safe on a single transaction.
type Tx interface {
	// TxOps contains the transaction operations.
	TxOps

	// Commit commits the transaction to storage.
	// Can return an error to indicate tx failure.
	Commit(ctx context.Context) error
	// Discard cancels the transaction.
	// If called after Commit, does nothing.
	// Cannot return an error.
	// Can be called unlimited times.
	Discard()
}
