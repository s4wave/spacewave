package hashmap

import (
	"github.com/Workiva/go-datastructures/trie/ctrie"
)

// CtrieMap implements a hash map with a ctrie.
type CtrieMap struct {
	ct *ctrie.Ctrie
}

// NewCtrieMap construts a new hashmap with a ctrie.
func NewCtrieMap() *CtrieMap {
	return &CtrieMap{
		ct: ctrie.New(nil),
	}
}

// Get looks up an item in the hash map.
func (m *CtrieMap) Get(key []byte) (interface{}, bool) {
	return m.ct.Lookup(key)
}

// Set sets an item in the hash map.
func (m *CtrieMap) Set(key []byte, value interface{}) {
	m.ct.Insert(key, value)
}

// Remove deletes an item from the hash map.
func (m *CtrieMap) Remove(key []byte) {
	m.ct.Remove(key)
}

// Exists checks if an item exists in the hash map.
func (m *CtrieMap) Exists(key []byte) bool {
	_, ok := m.ct.Lookup(key)
	return ok
}

// Iterate iterates over the hashmap.
//
// Iterator (might) not include items added during iteration.
func (m *CtrieMap) Iterate(cb func(key []byte, value interface{}) error) error {
	cancel := make(chan struct{})
	defer close(cancel)
	ents := m.ct.Iterator(cancel)
	for ent := range ents {
		if err := cb(ent.Key, ent.Value); err != nil {
			return err
		}
	}
	return nil
}

// _ is a type assertion
var _ Hashmap = ((*CtrieMap)(nil))
