package s4db

import (
	"bytes"

	"github.com/tidwall/btree"
)

// mentry is one memtable entry: a key's newest change since the last flush.
type mentry struct {
	// key is the key.
	key []byte
	// val is the value, a reference or a tombstone.
	val value
	// seq is the commit that made the change.
	seq uint64
}

// size estimates the entry's memory.
func (e mentry) size() int64 {
	return int64(len(e.key) + len(e.val.inline) + 64)
}

// lessMentry orders entries by key.
func lessMentry(a, b mentry) bool {
	return bytes.Compare(a.key, b.key) < 0
}

// memtable holds the entries since the last flush in key order. A published
// state's memtable is immutable; the writer copies it before changing it,
// which the tree does cheaply by sharing nodes.
type memtable struct {
	// tree holds the entries.
	tree *btree.BTreeG[mentry]
	// filter holds every key the memtable has held, so a lookup of another
	// key skips the tree. It only gains keys, so copies share it until a
	// flush starts a new one.
	filter *filter
	// bytes estimates the memory of the entries.
	bytes int64
}

// newMemtable returns an empty memtable whose filter is sized for budget
// bytes of entries.
func newMemtable(budget int64) *memtable {
	return &memtable{tree: btree.NewBTreeGOptions(lessMentry, btree.Options{NoLocks: true}), filter: newFilter(budget), bytes: 0}
}

// copy returns a memtable the caller may change without affecting m.
func (m *memtable) copy() *memtable {
	return &memtable{tree: m.tree.Copy(), filter: m.filter, bytes: m.bytes}
}

// get returns the entry of key.
func (m *memtable) get(key []byte) (mentry, bool) {
	if !m.filter.has(key) {
		return mentry{}, false
	}
	return m.tree.Get(mentry{key: key})
}

// set records e, replacing an earlier entry of its key.
func (m *memtable) set(e mentry) {
	m.filter.add(e.key)
	if old, ok := m.tree.Set(e); ok {
		m.bytes -= old.size()
	}
	m.bytes += e.size()
}

// dropThrough returns a memtable without the entries at or below seq, with a
// fresh filter: the flush through seq holds them now.
func (m *memtable) dropThrough(seq uint64, budget int64) *memtable {
	next := newMemtable(budget)
	m.tree.Scan(func(e mentry) bool {
		if e.seq > seq {
			next.set(e)
		}
		return true
	})
	return next
}

// len returns the number of entries.
func (m *memtable) len() int {
	return m.tree.Len()
}
