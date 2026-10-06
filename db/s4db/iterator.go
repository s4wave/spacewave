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
	entry() mentry
	// seek positions at the first key at or after k, or the last at or
	// before it when reverse; nil selects the first or last key.
	seek(k []byte, reverse bool)
	// step moves to the next key in the direction.
	step(reverse bool)
}

// mentry is an entry seen through a merged iterator.
type mentry struct {
	// del marks a deleted key.
	del bool
	// val is a stored value.
	val value
	// data is a buffered value, when buffered is set.
	data []byte
	// buffered selects data over val.
	buffered bool
}

// Iterator merges buffered changes, the overlay, and the index tree. The
// first layer holding a key decides its entry.
type Iterator struct {
	// p reads values.
	p *pager
	// layers holds the sources in priority order; the last is the tree.
	layers []layer
	// tree is the index tree layer.
	tree *pageIter
	// prefix bounds the keys.
	prefix []byte
	// reverse iterates in descending order.
	reverse bool
	// started is set once the iterator is positioned.
	started bool
	// ok reports whether the iterator is on an entry.
	ok bool
	// key is the current key.
	key []byte
	// cur is the current entry.
	cur mentry
	// err is the first error.
	err error
}

// newIterator returns an unpositioned iterator over st and changes.
func newIterator(p *pager, st *state, changes *btree.BTreeG[tentry], prefix []byte, reverse bool) *Iterator {
	// Buffered changes come first.
	it := &Iterator{p: p, prefix: prefix, reverse: reverse, tree: &pageIter{cursor{p: p, root: st.root}}}
	if changes != nil {
		it.layers = append(it.layers, &treeIter[tentry]{
			it:    changes.Iter(),
			keyOf: func(e tentry) []byte { return e.key },
			probe: func(k []byte) tentry { return tentry{key: k} },
			conv:  func(e tentry) mentry { return mentry{del: e.del, data: e.data, buffered: true} },
		})
	}

	// The overlay shadows the tree.
	it.layers = append(it.layers,
		&treeIter[overlayItem]{
			it:    st.overlay.Iter(),
			keyOf: func(it overlayItem) []byte { return it.key },
			probe: overlayProbe,
			conv:  func(it overlayItem) mentry { return mentry{del: it.e.del, val: it.e.val} },
		},
		it.tree,
	)
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
	return it.key
}

// Value returns the current value.
func (it *Iterator) Value() ([]byte, error) {
	if !it.Valid() {
		return nil, it.err
	}
	if it.cur.buffered {
		return it.cur.data, nil
	}
	return it.p.readValue(it.cur.val)
}

// ValueCopy copies the current value into b.
func (it *Iterator) ValueCopy(b []byte) ([]byte, error) {
	v, err := it.Value()
	if err != nil || v == nil {
		return nil, err
	}
	return append(b[:0], v...), nil
}

// Next advances the iterator.
func (it *Iterator) Next() bool {
	// The first call positions at the start.
	if it.err != nil {
		return false
	}
	if !it.started {
		return it.Seek(nil) == nil && it.ok
	}

	// Step past the current key.
	if !it.ok {
		return false
	}
	it.stepPast(it.key)
	it.settle()
	return it.Valid()
}

// Seek positions at the first key at or after k, or the last at or before it
// when reverse. A nil k selects the first or last key under the prefix.
func (it *Iterator) Seek(k []byte) error {
	// A nil key starts at the prefix bound.
	if it.err != nil {
		return it.err
	}
	it.started = true
	if k == nil && len(it.prefix) != 0 {
		k = it.prefix
		if it.reverse {
			k = prefixEnd(it.prefix)
		}
	}

	// Seek every layer and settle on the first live key.
	for _, l := range it.layers {
		l.seek(k, it.reverse)
	}
	it.settle()
	return it.err
}

// stepPast steps every layer on key.
func (it *Iterator) stepPast(key []byte) {
	for _, l := range it.layers {
		if l.valid() && bytes.Equal(l.key(), key) {
			l.step(it.reverse)
		}
	}
}

// settle moves to the next live key under the prefix from the layers'
// positions.
func (it *Iterator) settle() {
	for {
		// Stop at a tree read error or when every layer is done.
		best := it.front()
		it.err = it.tree.err
		if best == nil || it.err != nil {
			it.ok = false
			return
		}

		// Skip keys before the prefix and stop at keys after it.
		key := best.key()
		if !bytes.HasPrefix(key, it.prefix) {
			c := bytes.Compare(key, it.prefix)
			if (!it.reverse && c > 0) || (it.reverse && c < 0) {
				it.ok = false
				return
			}
			it.stepPast(key)
			continue
		}

		// Skip deleted keys.
		e := best.entry()
		if e.del {
			it.stepPast(key)
			continue
		}
		it.ok, it.key, it.cur = true, key, e
		return
	}
}

// front returns the layer on the next key in the direction, the first such
// layer on a tie, or nil when every layer is done.
func (it *Iterator) front() layer {
	var best layer
	for _, l := range it.layers {
		if !l.valid() {
			continue
		}
		if best == nil {
			best = l
			continue
		}
		c := bytes.Compare(l.key(), best.key())
		if (!it.reverse && c < 0) || (it.reverse && c > 0) {
			best = l
		}
	}
	return best
}

// Close releases the iterator.
func (it *Iterator) Close() {
	it.ok = false
	if it.err == nil {
		it.err = kvtx.ErrDiscarded
	}
}

// prefixEnd returns the smallest key above every key with prefix, or nil
// when none is.
func prefixEnd(prefix []byte) []byte {
	end := bytes.Clone(prefix)
	for i := len(end) - 1; i >= 0; i-- {
		if end[i] != 0xff {
			end[i]++
			return end[:i+1]
		}
	}
	return nil
}

// _ is a type assertion
var _ kvtx.Iterator = (*Iterator)(nil)
