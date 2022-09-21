package kvtx

import (
	"github.com/aperturerobotics/hydra/tx"
)

// Store is a transactional key/value store.
type Store interface {
	// NewTransaction returns a new transaction against the store.
	// Always call Discard() after you are done with the transaction.
	// The transaction will be read-only unless write is set.
	NewTransaction(write bool) (Tx, error)
}

// TxOps contains the database transaction operations.
type TxOps interface {
	// Get returns values for a key.
	Get(key []byte) (data []byte, found bool, err error)
	// Size returns the number of keys in the store.
	Size() (uint64, error)
	// Set sets the value of a key.
	// This will not be committed until Commit is called.
	Set(key, value []byte) error
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
	// ScanPrefixKeys iterates over keys only with a prefix.
	ScanPrefixKeys(prefix []byte, cb func(key []byte) error) error
	// Iterate returns an iterator with a given key prefix.
	//
	// Should always return non-nil, with error field filled if necessary.
	// If sort, iterates in sorted order, reverse reverses the key iteration.
	// The prefix is NOT clipped from the output keys.
	// If !sort, reverse has no effect.
	// Must call Next() or Seek() before valid.
	// Some implementations return BlockIterator.
	Iterate(prefix []byte, sort, reverse bool) Iterator
	// Exists checks if a key exists.
	Exists(key []byte) (bool, error)
}

// Iterator iterates over a kvtx Tx store with a given prefix.
// Note: Next() or Seek() must be called before iterator is valid.
type Iterator interface {
	// Err returns any error that has closed the iterator.
	// May return context.Canceled if closed.
	Err() error
	// Valid returns if the iterator points to a valid entry.
	//
	// If err is set, returns false.
	Valid() bool
	// Key returns the current entry key, or nil if not valid.
	Key() []byte
	// Value returns the current entry value, or nil if not valid.
	//
	// May cache the value between calls, copy if modifying.
	Value() []byte
	// ValueCopy copies the key to the given byte slice and returns it.
	// If the slice is not big enough (cap), it must create a new one and return it.
	// May use the value cached from Value() call as the source of the data.
	// May return nil if !Valid().
	ValueCopy([]byte) ([]byte, error)
	// Next advances to the next entry and returns Valid.
	Next() bool
	// Seek moves the iterator to the selected key, or the next key after the key.
	// Pass nil to seek to the beginning (or end if reversed).
	Seek(k []byte)
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
