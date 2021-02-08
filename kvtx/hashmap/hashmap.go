package hashmap

import "context"

// Hashmap implements an in-memory key/value store.
//
// Concurrency safe.
type Hashmap[V any] interface {
	// Get looks up an item in the hash map.
	// Returns value, found, error.
	Get(ctx context.Context, key []byte) (V, bool, error)
	// Set sets an item in the hash map.
	Set(ctx context.Context, key []byte, value V) error
	// Size returns the size of the hash map.
	Size(ctx context.Context) (uint64, error)
	// Delete deletes an item from the hash map.
	Delete(ctx context.Context, key []byte) error
	// Exists checks if an item exists in the hash map.
	Exists(ctx context.Context, key []byte) (bool, error)
	// Iterate iterates over the hashmap.
	//
	// Iterator (might) not include items added during iteration.
	Iterate(ctx context.Context, cb func(ctx context.Context, key []byte, value V) error) error
}

// NewHashmap constructs a new hash map of default type.
func NewHashmap[V any]() Hashmap[V] {
	return NewBTreeMap[V]()
}
