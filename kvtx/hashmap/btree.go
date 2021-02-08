package hashmap

import (
	"bytes"
	"context"

	"github.com/tidwall/btree"
)

// valType represents a key-value pair in the btree
type valType[V any] struct {
	key []byte
	val V
}

// valTypeLess implements the less function for btree
// may be called with nil if we pass nil for the pivot
func valTypeLess[V any](a, b *valType[V]) bool {
	var aKey, bKey []byte
	if a != nil {
		aKey = a.key
	}
	if b != nil {
		bKey = b.key
	}
	return bytes.Compare(aKey, bKey) < 0
}

// BTreeMap implements a hash map with a btree.
type BTreeMap[V any] struct {
	tree *btree.BTreeG[*valType[V]]
}

// NewBTreeMap constructs a new hashmap with a btree.
func NewBTreeMap[V any]() *BTreeMap[V] {
	return &BTreeMap[V]{
		tree: btree.NewBTreeG(valTypeLess[V]),
	}
}

// Get looks up an item in the hash map.
func (m *BTreeMap[V]) Get(ctx context.Context, key []byte) (val V, ok bool, err error) {
	item := &valType[V]{key: key}
	if found, exists := m.tree.Get(item); exists {
		return found.val, true, nil
	}
	return val, false, nil
}

// Size returns the number of keys in the map.
func (m *BTreeMap[V]) Size(ctx context.Context) (uint64, error) {
	return uint64(m.tree.Len()), nil
}

// Set sets an item in the hash map.
func (m *BTreeMap[V]) Set(ctx context.Context, key []byte, value V) error {
	item := &valType[V]{
		key: bytes.Clone(key),
		val: value,
	}
	m.tree.Set(item)
	return nil
}

// Delete deletes an item from the hash map.
func (m *BTreeMap[V]) Delete(ctx context.Context, key []byte) error {
	item := &valType[V]{key: key}
	m.tree.Delete(item)
	return nil
}

// Exists checks if an item exists in the hash map.
func (m *BTreeMap[V]) Exists(ctx context.Context, key []byte) (bool, error) {
	item := &valType[V]{key: key}
	_, exists := m.tree.Get(item)
	return exists, nil
}

// Iterate iterates over the hashmap.
//
// Iterator (might) not include items added during iteration.
//
// WARNING: Do not modify the btree (Set/Delete) during iteration as this will
// deadlock. Instead, collect the items to modify during iteration and modify
// them afterwards.
func (m *BTreeMap[V]) Iterate(ctx context.Context, cb func(ctx context.Context, key []byte, value V) error) error {
	m.tree.Ascend(nil, func(item *valType[V]) bool {
		if err := cb(ctx, item.key, item.val); err != nil {
			return false
		}
		return true
	})
	return nil
}

// _ is a type assertion
var _ Hashmap[any] = ((*BTreeMap[any])(nil))
