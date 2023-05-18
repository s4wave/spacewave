package hashmap

import (
	"context"

	"github.com/Workiva/go-datastructures/trie/ctrie"
)

// CtrieMap implements a hash map with a ctrie.
type CtrieMap[V any] struct {
	ct *ctrie.Ctrie
}

// NewCtrieMap construts a new hashmap with a ctrie.
func NewCtrieMap[V any]() *CtrieMap[V] {
	return &CtrieMap[V]{
		ct: ctrie.New(nil),
	}
}

func castValue[V any](vi any, iok bool) (val V, ok bool) {
	if iok {
		val, ok = vi.(V)
	}
	return
}

// Get looks up an item in the hash map.
func (m *CtrieMap[V]) Get(ctx context.Context, key []byte) (val V, ok bool, err error) {
	val, ok = castValue[V](m.ct.Lookup(key))
	return
}

// Size returns the number of keys in the map.
func (m *CtrieMap[V]) Size(ctx context.Context) (uint64, error) {
	return uint64(m.ct.Size()), nil
}

// Set sets an item in the hash map.
func (m *CtrieMap[V]) Set(ctx context.Context, key []byte, value V) error {
	m.ct.Insert(key, value)
	return nil
}

// Remove deletes an item from the hash map.
func (m *CtrieMap[V]) Delete(ctx context.Context, key []byte) error {
	m.ct.Remove(key)
	return nil
}

// Exists checks if an item exists in the hash map.
func (m *CtrieMap[V]) Exists(ctx context.Context, key []byte) (bool, error) {
	_, ok := m.ct.Lookup(key)
	return ok, nil
}

// Iterate iterates over the hashmap.
//
// Iterator (might) not include items added during iteration.
func (m *CtrieMap[V]) Iterate(ctx context.Context, cb func(ctx context.Context, key []byte, value V) error) error {
	cancel := make(chan struct{})
	defer close(cancel)
	ents := m.ct.Iterator(cancel)
	for ent := range ents {
		val, _ := castValue[V](ent.Value, true)
		if err := cb(ctx, ent.Key, val); err != nil {
			return err
		}
	}
	return nil
}

// _ is a type assertion
var _ Hashmap[any] = ((*CtrieMap[any])(nil))
