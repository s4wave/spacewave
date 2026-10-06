//go:build darwin || linux

package s4db

import (
	"bytes"
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/tidwall/btree"
)

// tentry is one change buffered in a write transaction.
type tentry struct {
	// key is the key.
	key []byte
	// del deletes the key.
	del bool
	// data is the new value.
	data []byte
}

// lessTEntry orders transaction entries by key.
func lessTEntry(a, b tentry) bool {
	return bytes.Compare(a.key, b.key) < 0
}

// newChanges returns an empty change buffer.
func newChanges() *btree.BTreeG[tentry] {
	return btree.NewBTreeGOptions(lessTEntry, btree.Options{NoLocks: true})
}

// Tx is a transaction on one snapshot. A write transaction buffers its
// changes and holds the writer lock.
type Tx struct {
	// db is the database.
	db *DB
	// st is the snapshot.
	st *state
	// stripe is the counter of st holding the snapshot.
	stripe int
	// changes buffers the writes of a write transaction; nil when read-only.
	changes *btree.BTreeG[tentry]
	// done is set after Commit or Discard.
	done bool
}

// Size returns the number of keys.
func (t *Tx) Size(ctx context.Context) (uint64, error) {
	// A finished transaction has no snapshot.
	if t.done {
		return 0, kvtx.ErrDiscarded
	}

	// Adjust the snapshot's count by each buffered change that adds or
	// removes a key.
	n := t.st.count
	var err error
	if t.changes != nil {
		t.changes.Scan(func(c tentry) bool {
			var found bool
			_, found, err = t.db.lookup(t.st, c.key)
			switch {
			case err != nil:
				return false
			case found && c.del:
				n--
			case !found && !c.del:
				n++
			}
			return true
		})
	}
	return uint64(max(n, 0)), err
}

// Get returns the value of key.
func (t *Tx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	// Check the transaction and key.
	switch {
	case t.done:
		return nil, false, kvtx.ErrDiscarded
	case len(key) == 0:
		return nil, false, kvtx.ErrEmptyKey
	}

	// A buffered change decides the key.
	if t.changes != nil {
		if c, ok := t.changes.Get(tentry{key: key}); ok {
			return c.data, !c.del, nil
		}
	}

	// Otherwise read the snapshot.
	v, found, err := t.db.lookup(t.st, key)
	if err != nil || !found {
		return nil, false, err
	}
	data, err := t.db.readValue(v)
	return data, err == nil, err
}

// Exists reports whether key is present.
func (t *Tx) Exists(ctx context.Context, key []byte) (bool, error) {
	switch {
	case t.done:
		return false, kvtx.ErrDiscarded
	case len(key) == 0:
		return false, kvtx.ErrEmptyKey
	}
	if t.changes != nil {
		if c, ok := t.changes.Get(tentry{key: key}); ok {
			return !c.del, nil
		}
	}
	_, found, err := t.db.lookup(t.st, key)
	return found, err
}

// Set buffers a write of key.
func (t *Tx) Set(ctx context.Context, key, value []byte) error {
	if err := t.writable(key); err != nil {
		return err
	}
	t.changes.Set(tentry{key: bytes.Clone(key), data: bytes.Clone(value)})
	return nil
}

// Delete buffers a deletion of key.
func (t *Tx) Delete(ctx context.Context, key []byte) error {
	if err := t.writable(key); err != nil {
		return err
	}
	t.changes.Set(tentry{key: bytes.Clone(key), del: true})
	return nil
}

// writable checks that the transaction may write key.
func (t *Tx) writable(key []byte) error {
	switch {
	case t.done:
		return kvtx.ErrDiscarded
	case t.changes == nil:
		return kvtx.ErrNotWrite
	case len(key) == 0:
		return kvtx.ErrEmptyKey
	case len(key) > MaxKeySize:
		return errors.Errorf("key of %d bytes exceeds %d", len(key), MaxKeySize)
	}
	return nil
}

// ScanPrefix calls cb with each key and value under prefix in key order.
func (t *Tx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	it := t.Iterate(ctx, prefix, true, false)
	defer it.Close()
	for it.Next() {
		v, err := it.Value()
		if err != nil {
			return err
		}
		if err := cb(it.Key(), v); err != nil {
			return err
		}
	}
	return it.Err()
}

// ScanPrefixKeys calls cb with each key under prefix in key order.
func (t *Tx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	it := t.Iterate(ctx, prefix, true, false)
	defer it.Close()
	for it.Next() {
		if err := cb(it.Key()); err != nil {
			return err
		}
	}
	return it.Err()
}

// Iterate returns an iterator over keys under prefix, always in key order.
func (t *Tx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	it := newIterator(t.db, t.st, t.changes, prefix, reverse)
	if t.done {
		it.err = kvtx.ErrDiscarded
	}
	return it
}

// Commit writes the changes with a full flush.
func (t *Tx) Commit(ctx context.Context) error {
	return t.commit(false)
}

// CommitOrdered writes the changes ordered after earlier commits without
// waiting for the drive to persist them.
func (t *Tx) CommitOrdered(ctx context.Context) error {
	return t.commit(true)
}

// commit finishes the transaction, writing its changes.
func (t *Tx) commit(ordered bool) error {
	// A read-only transaction has nothing to write.
	if t.done {
		return kvtx.ErrDiscarded
	}
	if t.changes == nil {
		t.Discard()
		return nil
	}

	// Collect the changes in key order.
	t.done = true
	db := t.db
	changes := make([]tentry, 0, t.changes.Len())
	t.changes.Scan(func(c tentry) bool {
		changes = append(changes, c)
		return true
	})

	// Write them, then run checkpoint and compaction work with the snapshot
	// released, and release the writer lock.
	err := db.commit(t.st, changes, ordered)
	db.release(t.st, t.stripe)
	if err == nil {
		err = db.afterCommit()
	}
	seq := db.cur.Load().seq
	db.unlockWriter()
	if err != nil || ordered {
		return err
	}

	// Flush outside the writer lock, sharing the flush with concurrent
	// commits.
	return db.syncThrough(seq)
}

// Discard ends the transaction without writing.
func (t *Tx) Discard() {
	if t.done {
		return
	}
	t.done = true
	t.db.release(t.st, t.stripe)
	if t.changes != nil {
		t.db.unlockWriter()
	}
}

// _ is a type assertion
var _ kvtx.OrderedCommitTx = (*Tx)(nil)

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
