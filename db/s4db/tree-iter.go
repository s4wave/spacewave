package s4db

import (
	"bytes"

	"github.com/tidwall/btree"
)

// treeIter iterates a tidwall B-tree of entries as a merge layer.
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

// valid reports whether the layer is on an entry.
func (l *treeIter[T]) valid() bool {
	return l.ok
}

// key returns the current key.
func (l *treeIter[T]) key() []byte {
	return l.keyOf(l.it.Item())
}

// entry returns the current entry.
func (l *treeIter[T]) entry() mentry {
	return l.conv(l.it.Item())
}

// seek positions at the first key at or after k, or the last at or before
// it when reverse; nil selects the first or last key.
func (l *treeIter[T]) seek(k []byte, reverse bool) {
	switch {
	case k == nil && reverse:
		l.ok = l.it.Last()
	case k == nil:
		l.ok = l.it.First()
	case !reverse:
		l.ok = l.it.Seek(l.probe(k))
	default:
		l.ok = l.it.Seek(l.probe(k))
		if !l.ok {
			l.ok = l.it.Last()
			return
		}
		if bytes.Compare(l.key(), k) > 0 {
			l.ok = l.it.Prev()
		}
	}
}

// step moves to the next key in the direction.
func (l *treeIter[T]) step(reverse bool) {
	if reverse {
		l.ok = l.it.Prev()
		return
	}
	l.ok = l.it.Next()
}

// _ is a type assertion
var _ layer = (*treeIter[tentry])(nil)
