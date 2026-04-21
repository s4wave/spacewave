package kvtx

import (
	"context"

	"github.com/s4wave/spacewave/db/tx"
)

// Store is a transactional key/value store.
type Store interface {
	// NewTransaction returns a new transaction against the store.
	// Always call Discard() after you are done with the transaction.
	// The transaction will be read-only unless write is set.
	NewTransaction(ctx context.Context, write bool) (Tx, error)
}

// TxOps contains the database transaction operations.
type TxOps interface {
	// Size returns the number of keys in the store.
	Size(ctx context.Context) (uint64, error)
	// Get returns values for a key.
	Get(ctx context.Context, key []byte) (data []byte, found bool, err error)
	// Exists checks if a key exists.
	Exists(ctx context.Context, key []byte) (bool, error)
	// Set sets the value of a key.
	// This will not be committed until Commit is called.
	Set(ctx context.Context, key, value []byte) error
	// Delete deletes a key.
	// This will not be committed until Commit is called.
	// Not found should not return an error.
	Delete(ctx context.Context, key []byte) error
	// ScanPrefix iterates over keys with a prefix.
	//
	// Note: neither key nor value should be retained outside cb() without
	// copying.
	//
	// Note: the ordering of the scan is not necessarily sorted.
	ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error
	// ScanPrefixKeys iterates over keys only with a prefix.
	ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error
	// Iterate returns an iterator with a given key prefix.
	//
	// Should always return non-nil, with error field filled if necessary.
	// If sort, iterates in sorted order, reverse reverses the key iteration.
	// The prefix is NOT clipped from the output keys.
	// If !sort, reverse MAY have no effect.
	// Must call Next() or Seek() before valid.
	// Some implementations return BlockIterator.
	// Context is used for the iterator and internally for iterator operations.
	// Return an ErrorIterator if anything goes wrong building the iterator.
	Iterate(ctx context.Context, prefix []byte, sort, reverse bool) Iterator
}

// Iterator iterates over a kvtx Tx store with a given prefix.
// Note: Next() or Seek() must be called before iterator is valid.
type Iterator interface {
	// Err returns any error that has closed the iterator.
	// May return context.Canceled or ErrDiscarded if closed.
	Err() error
	// Valid returns if the iterator points to a valid entry.
	//
	// If err is set, returns false.
	Valid() bool
	// Key returns the current entry key, or nil if not valid.
	//
	// NOTE: even if prefix is set this does not trim the prefix.
	Key() []byte
	// Value returns the current entry value, or nil if not valid.
	//
	// May cache the value between calls, copy if modifying.
	Value() ([]byte, error)
	// ValueCopy copies the value to the given byte slice and returns it.
	// If the slice is not big enough (cap), it must create a new one and return it.
	// May use the value cached from Value() call as the source of the data.
	// May return nil if !Valid().
	ValueCopy([]byte) ([]byte, error)
	// Next advances to the next entry and returns Valid.
	Next() bool
	// Seek moves the iterator to the first key >= the provided key (or <= in reverse mode).
	// Pass nil to seek to the beginning (or end if reversed).
	// It is not necessary to call Next() after seek.
	// If prefix is set, k should have the prefix or be nil. prefix is not prepended automatically.
	// Seek has three possible failure modes:
	//  - return an error without modifying the iterator
	//  - set the iterator Err to the error and return nil
	//  - set the iterator Err to the error and return the error
	Seek(k []byte) error
	// Close closes the iterator.
	// Note: it is not necessary to close all iterators before Discard().
	Close()
}

// Tx is a database transaction.
// Concurrent calls are not safe on a single transaction.
type Tx interface {
	// TxOps contains the transaction operations.
	TxOps

	// Tx contains the transaction confirm.
	tx.Tx
}
