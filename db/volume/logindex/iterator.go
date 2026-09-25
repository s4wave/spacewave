package logindex

import (
	"bytes"
	"context"

	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/tidwall/btree"
)

// iterator walks the keys with a prefix in one table snapshot.
type iterator struct {
	// it is the btree iterator.
	it btree.IterG[entry]
	// prefix bounds the keys.
	prefix []byte
	// prefixEnd is the smallest key above the prefix range when reversed, nil
	// when the range has no upper bound.
	prefixEnd []byte
	// reverse walks the keys in descending order.
	reverse bool

	// err closes the iterator.
	err error
	// started is set once Next or Seek positions the iterator.
	started bool
	// valid is set while the iterator is on a key in the prefix range.
	valid bool
}

// newIterator returns an iterator over the keys of tree with prefix.
func newIterator(tree *table, prefix []byte, reverse bool) *iterator {
	i := &iterator{it: tree.Iter(), prefix: prefix, reverse: reverse}
	if reverse {
		i.prefixEnd = prefixUpperBound(prefix)
	}
	return i
}

// Err returns the error that closed the iterator.
func (i *iterator) Err() error {
	return i.err
}

// Valid reports whether the iterator is on an entry.
func (i *iterator) Valid() bool {
	return i.err == nil && i.valid
}

// Key returns the current key, or nil when not valid.
func (i *iterator) Key() []byte {
	if !i.Valid() {
		return nil
	}
	return i.it.Item().key
}

// Value returns the current value, or nil when not valid.
func (i *iterator) Value() ([]byte, error) {
	if !i.Valid() {
		return nil, i.err
	}
	return i.it.Item().value, nil
}

// ValueCopy appends the current value to bt[:0].
func (i *iterator) ValueCopy(bt []byte) ([]byte, error) {
	if !i.Valid() {
		return nil, i.err
	}
	return append(bt[:0], i.it.Item().value...), nil
}

// Next advances to the next entry and reports Valid.
func (i *iterator) Next() bool {
	if i.err != nil {
		return false
	}
	var ok bool
	switch {
	case !i.started:
		ok = i.seekBoundary()
		i.started = true
	case !i.valid:
		return false
	case i.reverse:
		ok = i.it.Prev()
	default:
		ok = i.it.Next()
	}
	return i.bound(ok)
}

// Seek moves to the first key at or after k, or at or before k when
// reversed. A nil k seeks to the start of the range.
func (i *iterator) Seek(k []byte) error {
	if i.err != nil {
		return i.err
	}
	i.started = true
	if len(k) == 0 {
		i.bound(i.seekBoundary())
		return nil
	}
	if !i.reverse {
		if bytes.Compare(k, i.prefix) < 0 {
			k = i.prefix
		}
		i.bound(i.it.Seek(entry{key: k}))
		return nil
	}
	i.bound(i.seekReverse(k))
	return nil
}

// seekBoundary moves to the first entry of the range in walk order.
func (i *iterator) seekBoundary() bool {
	if !i.reverse {
		return i.it.Seek(entry{key: i.prefix})
	}
	if len(i.prefixEnd) == 0 {
		return i.it.Last()
	}
	if !i.it.Seek(entry{key: i.prefixEnd}) {
		return i.it.Last()
	}
	return i.it.Prev()
}

// seekReverse moves to the last entry at or before k.
func (i *iterator) seekReverse(k []byte) bool {
	if bytes.Compare(k, i.prefix) < 0 {
		return false
	}
	if len(i.prefixEnd) != 0 && bytes.Compare(k, i.prefixEnd) >= 0 {
		return i.seekBoundary()
	}
	if !i.it.Seek(entry{key: k}) {
		return i.it.Last()
	}
	if bytes.Compare(i.it.Item().key, k) > 0 {
		return i.it.Prev()
	}
	return true
}

// bound sets valid from a move's result and the prefix range, and reports it.
func (i *iterator) bound(ok bool) bool {
	i.valid = ok && bytes.HasPrefix(i.it.Item().key, i.prefix)
	return i.valid
}

// Close closes the iterator.
func (i *iterator) Close() {
	i.valid = false
	i.it.Release()
	if i.err == nil {
		i.err = context.Canceled
	}
}

// prefixUpperBound returns the smallest key above the contiguous prefix range.
// A nil result means the prefix range has no finite upper bound.
func prefixUpperBound(prefix []byte) []byte {
	upper := bytes.Clone(prefix)
	for idx := len(upper) - 1; idx >= 0; idx-- {
		if upper[idx] != 0xff {
			upper[idx]++
			return upper[:idx+1]
		}
	}
	return nil
}

// _ is a type assertion
var _ kvtx.Iterator = (*iterator)(nil)
