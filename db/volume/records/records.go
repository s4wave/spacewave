// Package records defines the ordered record store an IndexedDB object store
// provides, with a memory implementation that simulates power loss.
//
// Every call is one IndexedDB transaction and one round trip. A commit is
// atomic. A durable commit maps to the strict durability hint and returns once
// it and every earlier commit are durable; any other commit maps to the relaxed
// hint, and a crash may lose it along with every later commit.
package records

import "context"

// Op is one change of a commit.
type Op struct {
	// Key is the record key.
	Key []byte
	// Value is the record a put stores.
	Value []byte
	// Delete deletes the record.
	Delete bool
}

// Store is an ordered record store.
type Store interface {
	// Get reads the records of keys, nil for an absent key.
	Get(ctx context.Context, keys [][]byte) ([][]byte, error)
	// Has reports which keys have records without reading them.
	Has(ctx context.Context, keys [][]byte) ([]bool, error)
	// Scan calls fn with each record whose key has prefix, in key order. fn
	// may retain the key and value.
	Scan(ctx context.Context, prefix []byte, fn func(key, value []byte) error) error
	// Commit applies ops atomically in order, durably if durable is set. The
	// store does not retain ops.
	Commit(ctx context.Context, ops []Op, durable bool) error
}
