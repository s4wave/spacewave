package s4db

import (
	"bytes"

	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/tidwall/btree"
)

// layer is one sorted source of entries in a merged iterator.
type layer interface {
	// valid reports whether the layer is on an entry.
	valid() bool
	// key returns the current key.
	key() []byte
	// entry returns the current entry.
	entry() ientry
	// seek positions at the first key at or after k, or the last at or
	// before it when reverse; nil selects the first or last key.
	seek(k []byte, reverse bool)
	// step moves to the next key in the direction.
	step(reverse bool)
}

// ientry is an entry seen through a merged iterator.
type ientry struct {
	// del marks a deleted key.
	del bool
	// val is a stored value.
	val value
	// data is a buffered value, when buffered is set.
	data []byte
	// buffered selects data over val.
	buffered bool
}

// Iterator merges buffered changes, the memtable, and the runs. The first
// layer holding a key decides its entry; a tombstone hides the key.
type Iterator struct {
	// db reads values.
	db *DB
	// layers holds the sources in priority order: the changes, the
	// memtable, then the runs newest first.
	layers []layer
	// st is the snapshot the memtable and run layers read.
	st *state
	// prefix bounds the keys.
	prefix []byte
	// reverse iterates in descending order.
	reverse bool
	// started is set once the iterator is positioned.
	started bool
	// ok reports whether the iterator is on an entry.
	ok bool
	// cur is the current key.
	cur []byte
	// ent is the current entry.
	ent ientry
	// err is the first error.
	err error
}

// newIterator returns an unpositioned iterator over st and changes.
func newIterator(db *DB, st *state, changes *btree.BTreeG[tentry], prefix []byte, reverse bool) *Iterator {
	// Buffered changes come first.
	it := &Iterator{db: db, st: st, prefix: prefix, reverse: reverse}
	if changes != nil {
		it.layers = append(it.layers, &treeIter[tentry]{
			it:    changes.Iter(),
			keyOf: func(e tentry) []byte { return e.key },
			probe: func(k []byte) tentry { return tentry{key: k} },
			conv:  func(e tentry) ientry { return ientry{del: e.del, data: e.data, buffered: true} },
		})
	}

	// The memtable shadows the runs.
	it.layers = append(it.layers, &treeIter[mentry]{
		it:    st.mem.tree.Iter(),
		keyOf: func(e mentry) []byte { return e.key },
		probe: func(k []byte) mentry { return mentry{key: k} },
		conv:  func(e mentry) ientry { return ientry{del: e.val.kind == valueDeleted, val: e.val} },
	})
	it.layers = append(it.layers, db.runLayers(st, prefix)...)
	return it
}

// Err returns the first error.
func (it *Iterator) Err() error {
	return it.err
}

// Valid reports whether the iterator is on an entry.
func (it *Iterator) Valid() bool {
	return it.err == nil && it.ok
}

// Key returns the current key.
func (it *Iterator) Key() []byte {
	if !it.Valid() {
		return nil
	}
	return it.cur
}

// Value returns the current value.
func (it *Iterator) Value() ([]byte, error) {
	if !it.Valid() {
		return nil, it.err
	}
	if it.ent.buffered {
		return it.ent.data, nil
	}
	return it.db.readValue(it.ent.val)
}

// ValueCopy copies the current value into buf.
func (it *Iterator) ValueCopy(buf []byte) ([]byte, error) {
	v, err := it.Value()
	if err != nil || v == nil {
		return nil, err
	}
	return append(buf[:0], v...), nil
}

// Next advances to the next entry, or positions at the first when the
// iterator has not started.
func (it *Iterator) Next() bool {
	// Position at the start the first time.
	if it.err != nil {
		return false
	}
	if !it.started {
		return it.Seek(nil) == nil && it.ok
	}

	// Move every layer past the current key and settle on the next.
	if !it.ok {
		return false
	}
	it.stepPast(it.cur)
	return it.settle()
}

// Seek positions the iterator at the first key at or after k, or at or
// before it in reverse; nil seeks to the start or the end of the prefix.
func (it *Iterator) Seek(k []byte) error {
	// Clip the seek to the prefix.
	if it.err != nil {
		return it.err
	}
	it.started = true
	target := k
	if k == nil || (!it.reverse && bytes.Compare(k, it.prefix) < 0) {
		target = it.prefix
		if it.reverse && k == nil {
			target = nil
		}
	}
	if it.reverse && target == nil && len(it.prefix) != 0 {
		target = prefixEnd(it.prefix)
	}

	// Position every layer.
	for _, l := range it.layers {
		l.seek(target, it.reverse)
	}

	// A reverse seek to the prefix's end may land on the end key itself,
	// which lies outside the prefix; step past it, then settle.
	if it.reverse && target != nil {
		for _, l := range it.layers {
			for l.valid() && bytes.Compare(l.key(), target) >= 0 && !bytes.HasPrefix(l.key(), it.prefix) {
				l.step(true)
			}
		}
	}
	it.settle()
	return nil
}

// prefixEnd returns the smallest key above every key with prefix p, or nil
// when none exists.
func prefixEnd(p []byte) []byte {
	end := bytes.Clone(p)
	for i := len(end) - 1; i >= 0; i-- {
		if end[i] != 0xff {
			end[i]++
			return end[:i+1]
		}
	}
	return nil
}

// stepPast moves every layer standing on key past it.
func (it *Iterator) stepPast(key []byte) {
	for _, l := range it.layers {
		if l.valid() && bytes.Equal(l.key(), key) {
			l.step(it.reverse)
		}
	}
}

// settle finds the next key in the direction: the smallest (or largest)
// key any layer stands on, decided by the first layer holding it, skipping
// deletions and stopping at the prefix's end.
func (it *Iterator) settle() bool {
	for {
		// Find the frontier key and the layer that decides it.
		it.ok = false
		var best []byte
		var ent ientry
		for _, l := range it.layers {
			if !l.valid() {
				continue
			}
			k := l.key()
			c := 1
			if best != nil {
				c = bytes.Compare(k, best)
				if it.reverse {
					c = -c
				}
			}
			if best == nil || c < 0 {
				best, ent = k, l.entry()
			}
		}
		if best == nil || !bytes.HasPrefix(best, it.prefix) {
			return false
		}

		// Skip a deleted key, else stand on it.
		if ent.del {
			it.stepPast(best)
			continue
		}
		it.cur, it.ent, it.ok = best, ent, true
		return true
	}
}

// Close releases the iterator.
func (it *Iterator) Close() {
	it.layers = nil
	it.ok = false
}

// _ is a type assertion
var _ kvtx.Iterator = (*Iterator)(nil)
