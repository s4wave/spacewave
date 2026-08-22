//go:build !js && !wasip1

package store_kvtx_bolt

import (
	"bytes"

	"github.com/s4wave/spacewave/db/kvtx"
)

// kvEntry is one materialized key/value entry in a merged view.
type kvEntry struct {
	key   []byte
	value []byte
}

// sliceIterator iterates over a materialized, sorted set of entries.
type sliceIterator struct {
	entries []kvEntry
	reverse bool
	idx     int
}

// newSliceIterator constructs an iterator over the entries. The entries
// must already be sorted forward; reverse iteration walks them backwards.
func newSliceIterator(entries []kvEntry, reverse bool) *sliceIterator {
	return &sliceIterator{entries: entries, reverse: reverse, idx: -1}
}

// Err returns any error that has closed the iterator.
func (i *sliceIterator) Err() error { return nil }

// Valid returns if the iterator points to a valid entry.
func (i *sliceIterator) Valid() bool {
	return i.idx >= 0 && i.idx < len(i.entries)
}

// Key returns the current entry key, or nil if not valid.
func (i *sliceIterator) Key() []byte {
	if !i.Valid() {
		return nil
	}
	return i.entries[i.idx].key
}

// Value returns the current entry value, or nil if not valid.
func (i *sliceIterator) Value() ([]byte, error) {
	if !i.Valid() {
		return nil, nil
	}
	return i.entries[i.idx].value, nil
}

// ValueCopy copies the value to the given byte slice and returns it.
func (i *sliceIterator) ValueCopy(bt []byte) ([]byte, error) {
	if !i.Valid() {
		return nil, nil
	}
	return append(bt[:0], i.entries[i.idx].value...), nil
}

// Next advances to the next entry and returns Valid.
func (i *sliceIterator) Next() bool {
	if !i.Valid() && i.idx != -1 {
		return false
	}
	i.idx++
	if !i.Valid() && i.idx == 0 && len(i.entries) != 0 {
		// First Next from a fresh iterator starts at the beginning
		// (forward) or end (reverse).
		if i.reverse {
			i.idx = len(i.entries) - 1
		} else {
			i.idx = 0
		}
	}
	return i.Valid()
}

// Seek moves the iterator to the first key >= k, or <= k in reverse mode.
// Pass nil to seek to the beginning (or end if reversed).
func (i *sliceIterator) Seek(k []byte) error {
	if len(i.entries) == 0 {
		i.idx = -1
		return nil
	}
	if len(k) == 0 {
		if i.reverse {
			i.idx = len(i.entries) - 1
		} else {
			i.idx = 0
		}
		return nil
	}
	lo, hi := 0, len(i.entries)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		cmp := stringCompare(i.entries[mid].key, k)
		fwd := cmp >= 0
		if i.reverse {
			fwd = cmp <= 0
		}
		if fwd {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	i.idx = lo
	if i.reverse && i.idx == len(i.entries) {
		i.idx = len(i.entries) - 1
	}
	return nil
}

// Close closes the iterator.
func (i *sliceIterator) Close() {
	i.idx = -1
	i.entries = nil
}

// _ is a type assertion
var _ kvtx.Iterator = (*sliceIterator)(nil)

// stringCompare compares two byte slices lexicographically.
func stringCompare(a, b []byte) int { return bytes.Compare(a, b) }
