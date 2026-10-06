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

// treeIter iterates a tidwall B-tree of entries.
type treeIter[T any] struct {
	// it is the underlying iterator.
	it btree.IterG[T]
	// ok reports whether it is on an item.
	ok bool
	// keyOf returns an item's key.
	keyOf func(T) []byte
	// probe returns an item with key k for seeking.
	probe func(k []byte) T
	// conv converts an item to a merged entry.
	conv func(T) mentry
}

func (l *treeIter[T]) valid() bool   { return l.ok }
func (l *treeIter[T]) key() []byte   { return l.keyOf(l.it.Item()) }
func (l *treeIter[T]) entry() mentry { return l.conv(l.it.Item()) }

func (l *treeIter[T]) seek(k []byte, reverse bool) {
	switch {
	case k == nil && reverse:
		l.ok = l.it.Last()
	case k == nil:
		l.ok = l.it.First()
	case !reverse:
		l.ok = l.it.Seek(l.probe(k))
	default:
		if l.ok = l.it.Seek(l.probe(k)); !l.ok {
			l.ok = l.it.Last()
		} else if bytes.Compare(l.key(), k) > 0 {
			l.ok = l.it.Prev()
		}
	}
}

func (l *treeIter[T]) step(reverse bool) {
	if reverse {
		l.ok = l.it.Prev()
	} else {
		l.ok = l.it.Next()
	}
}

// pageIter iterates the index tree.
type pageIter struct {
	cursor
}

func (l *pageIter) valid() bool                 { return l.cursor.valid() }
func (l *pageIter) key() []byte                 { return l.cursor.key() }
func (l *pageIter) entry() mentry               { return mentry{val: l.cursor.value()} }
func (l *pageIter) seek(k []byte, reverse bool) { l.cursor.seek(k, reverse) }
func (l *pageIter) step(reverse bool)           { l.cursor.step(reverse) }

// Iterator merges buffered changes, the overlay, and the index tree. The
// first layer holding a key decides its entry.
type Iterator struct {
	// db is the database.
	db *DB
	// layers holds the sources in priority order.
	layers []layer
	// prefix bounds the keys.
	prefix []byte
	// reverse iterates in descending order.
	reverse bool
	// started is set once the iterator is positioned.
	started bool
	// ok reports whether the iterator is on an entry.
	ok bool
	// curKey and cur are the current entry.
	curKey []byte
	cur    value
	// curEntry is the current merged entry.
	curEntry mentry
	// err is the first error.
	err error
}

// newIterator returns an unpositioned iterator over st and changes.
func newIterator(db *DB, st *state, changes *btree.BTreeG[tentry], prefix []byte, reverse bool) *Iterator {
	it := &Iterator{db: db, prefix: prefix, reverse: reverse}
	if changes != nil {
		it.layers = append(it.layers, &treeIter[tentry]{
			it:    changes.Iter(),
			keyOf: func(e tentry) []byte { return e.key },
			probe: func(k []byte) tentry { return tentry{key: k} },
			conv:  func(e tentry) mentry { return mentry{del: e.del, data: e.data, buffered: true} },
		})
	}
	it.layers = append(it.layers,
		&treeIter[*oentry]{
			it:    st.overlay.Iter(),
			keyOf: func(e *oentry) []byte { return e.key },
			probe: func(k []byte) *oentry { return &oentry{key: k} },
			conv:  func(e *oentry) mentry { return mentry{del: e.del, val: e.val} },
		},
		&pageIter{cursor{p: db, root: st.root}},
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
	return it.curKey
}

// Value returns the current value.
func (it *Iterator) Value() ([]byte, error) {
	if !it.Valid() {
		return nil, it.err
	}
	if it.curEntry.buffered {
		return it.curEntry.data, nil
	}
	return it.db.readValue(it.cur)
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
	it.stepPast(it.curKey)
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
		if pi, ok := it.layers[len(it.layers)-1].(*pageIter); ok && pi.err != nil {
			it.err = pi.err
		}
		if best == nil || it.err != nil {
			it.ok = false
			return
		}
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
		e := best.entry()
		if e.del {
			it.stepPast(key)
			continue
		}
		it.ok, it.curKey, it.cur, it.curEntry = true, key, e.val, e
		return
	}
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
