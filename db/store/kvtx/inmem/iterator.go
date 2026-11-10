package store_kvtx_inmem

import (
	"bytes"
	"context"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/tidwall/btree"
)

// Iterator implements the kvtx.Iterator interface.
//
// It merges two cursors without copying entries: one over the committed tree
// and, in a write transaction, one over the transaction's added values, with
// committed keys the transaction deleted or replaced skipped. Opening takes an
// O(1) copy-on-write snapshot of each tree, so the iterator sees the
// transaction as it was when it opened, holds no tree lock, and costs nothing
// per entry it never reaches.
type Iterator struct {
	// ctx is the context
	ctx context.Context
	// tx is the transaction
	tx *Tx
	// prefix is the key prefix to filter by
	prefix []byte
	// reverse indicates if iteration should be reversed
	reverse bool
	// err is any error that occurred
	err error
	// base walks the committed tree.
	base cursor
	// added walks the transaction's added values, unset for a reader.
	added cursor
	// deleted holds the committed keys the transaction deleted, nil for a reader.
	deleted *btree.BTreeG[*valType]
	// started is set once the iterator has been positioned.
	started bool
	// cur is the current entry, nil before the start and after the end.
	cur *valType
	// released indicates if the iterator was closed
	released atomic.Bool
}

// cursor is one tree's position in the merge.
type cursor struct {
	iter btree.IterG[*valType]
	// ok is set while iter points at an entry under the prefix.
	ok bool
}

// item returns the cursor's entry, or nil when it has none.
func (c *cursor) item() *valType {
	if !c.ok {
		return nil
	}
	return c.iter.Item()
}

// NewIterator constructs a new iterator. Entries come in key order whether or
// not sort is set.
func NewIterator(
	ctx context.Context,
	tx *Tx,
	prefix []byte,
	sort bool,
	reverse bool,
) *Iterator {
	// Construct the iterator with its transaction and traversal options.
	it := &Iterator{
		ctx:     ctx,
		tx:      tx,
		prefix:  prefix,
		reverse: reverse,
	}
	if tx.discarded.Load() {
		it.err = kvtx.ErrDiscarded
		return it
	}

	// Snapshot each tree the merge reads.
	it.base.iter = tx.s.tree.Copy().Iter()
	if tx.write {
		it.added.iter = tx.added.Copy().Iter()
		it.deleted = tx.deleted.Copy()
	}
	return it
}

// inPrefix reports whether item is under the iterator's prefix.
func (it *Iterator) inPrefix(item *valType) bool {
	return bytes.HasPrefix(item.key, it.prefix)
}

// before reports whether a comes before b in traversal order.
func (it *Iterator) before(a, b *valType) bool {
	if it.reverse {
		return bytes.Compare(a.key, b.key) > 0
	}
	return bytes.Compare(a.key, b.key) < 0
}

// step moves c one entry in traversal order.
func (it *Iterator) step(c *cursor) {
	if it.reverse {
		c.ok = c.iter.Prev()
	} else {
		c.ok = c.iter.Next()
	}
	c.ok = c.ok && it.inPrefix(c.iter.Item())
}

// seekCursor positions c at the first entry at or after k in traversal order.
// A nil k is the start of the prefix.
func (it *Iterator) seekCursor(c *cursor, k []byte) {
	if !it.reverse {
		if k == nil || bytes.Compare(k, it.prefix) < 0 {
			k = it.prefix
		}
		c.ok = c.iter.Seek(&valType{key: k})
	} else {
		if end := prefixEnd(it.prefix); k == nil || end != nil && bytes.Compare(k, end) > 0 {
			k = end
		}
		switch {
		case k == nil:
			c.ok = c.iter.Last()
		case !c.iter.Seek(&valType{key: k}):
			c.ok = c.iter.Last()
		case bytes.Equal(c.iter.Item().key, k) && it.inPrefix(c.iter.Item()):
			c.ok = true
		default:
			c.ok = c.iter.Prev()
		}
	}
	c.ok = c.ok && it.inPrefix(c.iter.Item())
}

// prefixEnd returns the first key after every key under prefix, or nil when
// no such key exists.
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

// settle sets cur to the merge's next entry: the earlier of the two cursors'
// entries, where an added value shadows the committed one under its key and a
// deleted committed key is skipped.
func (it *Iterator) settle() {
	for {
		b, a := it.base.item(), it.added.item()
		if b != nil && a != nil && bytes.Equal(b.key, a.key) {
			it.step(&it.base)
			continue
		}
		if b != nil && (a == nil || it.before(b, a)) {
			if it.deleted != nil {
				if _, gone := it.deleted.Get(b); gone {
					it.step(&it.base)
					continue
				}
			}
			it.cur = b
			return
		}
		it.cur = a
		return
	}
}

// seek positions both cursors at k and settles the merge.
func (it *Iterator) seek(k []byte) {
	it.started = true
	it.seekCursor(&it.base, k)
	if it.tx.write {
		it.seekCursor(&it.added, k)
	}
	it.settle()
}

// isReleased returns if the iterator or transaction was released.
func (it *Iterator) isReleased() bool {
	return it.released.Load() || it.tx.discarded.Load()
}

// Err returns any error that has closed the iterator.
func (it *Iterator) Err() error {
	if it.err != nil {
		return it.err
	}
	if it.isReleased() {
		return kvtx.ErrDiscarded
	}
	if it.ctx.Err() != nil {
		return context.Canceled
	}
	return nil
}

// Valid returns if the iterator points to a valid entry. The context is
// checked when the iterator moves, not here, so an entry Next or Seek reached
// stays readable until the next move.
func (it *Iterator) Valid() bool {
	return it.err == nil && !it.isReleased() && it.cur != nil
}

// checkMove records a canceled context as the iterator error before a move,
// and returns the iterator error.
func (it *Iterator) checkMove() error {
	if it.err == nil && it.ctx.Err() != nil {
		it.err = context.Canceled
	}
	return it.Err()
}

// Key returns the current entry key.
func (it *Iterator) Key() []byte {
	if !it.Valid() {
		return nil
	}
	return it.cur.key
}

// Value returns the current entry value.
func (it *Iterator) Value() ([]byte, error) {
	if !it.Valid() {
		return nil, nil
	}
	return it.cur.val, nil
}

// ValueCopy copies the value to the given byte slice and returns it.
func (it *Iterator) ValueCopy(dst []byte) ([]byte, error) {
	// Read the iterator value before preparing its destination buffer.
	val, err := it.Value()
	if err != nil || val == nil {
		return nil, err
	}

	// Size the destination buffer to hold the iterator value.
	if cap(dst) < len(val) {
		dst = make([]byte, len(val))
	} else {
		dst = dst[:len(val)]
	}

	// Copy the iterator value into the caller buffer.
	copy(dst, val)
	return dst, nil
}

// Next advances to the next entry.
func (it *Iterator) Next() bool {
	if it.checkMove() != nil {
		return false
	}
	switch {
	case !it.started:
		it.seek(nil)
	case it.cur != nil:
		// The added value shadows the committed one, so the base cursor
		// moves past the key only when it points at it.
		if b := it.base.item(); b != nil && bytes.Equal(b.key, it.cur.key) {
			it.step(&it.base)
		}
		if a := it.added.item(); a != nil && bytes.Equal(a.key, it.cur.key) {
			it.step(&it.added)
		}
		it.settle()
	}
	return it.Valid()
}

// Seek moves the iterator to the first key >= the provided key (or <= in
// reverse mode). An empty key seeks to the first entry in traversal order.
func (it *Iterator) Seek(k []byte) error {
	if err := it.checkMove(); err != nil {
		return err
	}
	if len(k) == 0 {
		k = nil
	}
	it.seek(k)
	return nil
}

// Close closes the iterator.
func (it *Iterator) Close() {
	if !it.released.Swap(true) {
		it.base.iter.Release()
		it.added.iter.Release()
		it.base.ok, it.added.ok = false, false
		it.cur = nil
		it.err = kvtx.ErrDiscarded
	}
}

// _ is a type assertion
var _ kvtx.Iterator = (*Iterator)(nil)
