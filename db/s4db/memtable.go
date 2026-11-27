package s4db

import (
	"bytes"
	"sync"

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
func lessMentry(a, b *mentry) bool {
	return bytes.Compare(a.key, b.key) < 0
}

// memtable holds the entries since the last flush in key order. A published
// state's memtable is immutable. The writer copies it before changing it,
// which the tree does cheaply by sharing nodes. The tree holds pointers, so a
// commit's copy of each node it changes moves a word per entry rather than
// the entry. An entry never changes once set, so trees share it.
type memtable struct {
	// copyMtx serializes Copy, which changes the source tree's COW epoch.
	// Readers never use that epoch and never hold this lock.
	copyMtx sync.Mutex
	// tree holds the entries.
	tree *btree.BTreeG[*mentry]
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
	// Copy changes source metadata even though its nodes remain immutable.
	// Only competing copies serialize. A scan cannot hold up this
	// operation.
	m.copyMtx.Lock()
	defer m.copyMtx.Unlock()
	return &memtable{tree: m.tree.Copy(), filter: m.filter, bytes: m.bytes}
}

// probes holds the entries get searches with. The tree's comparison takes its
// probe by pointer through a function value, so a probe built per call escapes.
// A pooled probe makes a lookup allocate nothing.
var probes = sync.Pool{New: func() any { return new(mentry) }}

// get returns the entry of key.
func (m *memtable) get(key []byte) (mentry, bool) {
	// Skip the tree for a key the filter has never held.
	if !m.filter.has(key) {
		return mentry{}, false
	}

	// Search with a pooled probe, cleared before it goes back so the pool
	// keeps no caller's key alive.
	probe := probes.Get().(*mentry)
	probe.key = key
	e, ok := m.tree.Get(probe)
	probe.key = nil
	probes.Put(probe)
	if !ok {
		return mentry{}, false
	}
	return *e, true
}

// set records e, replacing an earlier entry of its key.
func (m *memtable) set(e mentry) {
	m.setEntry(&e)
}

// setEntry records e, which no one changes afterward, replacing an earlier
// entry of its key.
func (m *memtable) setEntry(e *mentry) {
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
	m.tree.Scan(func(e *mentry) bool {
		if e.seq > seq {
			next.setEntry(e)
		}
		return true
	})
	return next
}

// len returns the number of entries.
func (m *memtable) len() int {
	return m.tree.Len()
}

// scan calls fn with each entry in key order until fn returns false.
func (m *memtable) scan(fn func(e *mentry) bool) {
	m.tree.Scan(fn)
}

// layer returns an iterator layer over the entries, which shadow the runs.
func (m *memtable) layer() *treeIter[*mentry] {
	return &treeIter[*mentry]{
		it:    m.tree.Iter(),
		keyOf: func(e *mentry) []byte { return e.key },
		probe: func(k []byte) *mentry { return &mentry{key: k} },
		conv:  func(e *mentry) ientry { return ientry{del: e.val.kind == valueDeleted, val: e.val} },
	}
}

// changedUnder reports whether an entry under prefix changed after commit
// after.
func (m *memtable) changedUnder(prefix []byte, after uint64) bool {
	changed := false
	m.tree.Ascend(&mentry{key: prefix}, func(e *mentry) bool {
		if !bytes.HasPrefix(e.key, prefix) {
			return false
		}
		changed = e.seq > after
		return !changed
	})
	return changed
}
