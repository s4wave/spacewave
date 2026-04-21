//go:build js

package store_kvtx_opfs

import (
	"bytes"
	"context"
	"sort"

	"github.com/s4wave/spacewave/db/kvtx"
)

// Iterator iterates over OPFS kvtx entries.
type Iterator struct {
	tx      *Tx
	entries []kvEntry
	pos     int
	err     error
	reverse bool
}

// Err returns any error that has closed the iterator.
func (it *Iterator) Err() error {
	return it.err
}

// Valid returns if the iterator points to a valid entry.
func (it *Iterator) Valid() bool {
	return it.err == nil && it.pos >= 0 && it.pos < len(it.entries)
}

// Key returns the current entry key.
func (it *Iterator) Key() []byte {
	if !it.Valid() {
		return nil
	}
	return it.entries[it.pos].key
}

// Value returns the current entry value, loading it on demand.
func (it *Iterator) Value() ([]byte, error) {
	if !it.Valid() {
		return nil, it.err
	}
	e := &it.entries[it.pos]
	if e.value != nil {
		return e.value, nil
	}

	// Check write buffer first.
	if it.tx.write {
		if val, ok := it.tx.sets[e.encoded]; ok {
			e.value = val
			return val, nil
		}
	}

	// Load from OPFS.
	shard := shardPrefix(e.encoded)
	shardDir, err := it.tx.getShardDir(shard, false)
	if err != nil {
		it.err = err
		return nil, err
	}
	data, err := it.tx.readFile(shardDir, e.encoded)
	if err != nil {
		it.err = err
		return nil, err
	}
	e.value = data
	return data, nil
}

// ValueCopy copies the value to the given byte slice.
func (it *Iterator) ValueCopy(dst []byte) ([]byte, error) {
	if !it.Valid() {
		return nil, it.err
	}
	val, err := it.Value()
	if err != nil {
		return nil, err
	}
	return append(dst[:0], val...), nil
}

// Next advances to the next entry in iteration direction.
func (it *Iterator) Next() bool {
	if it.err != nil {
		return false
	}
	if it.reverse {
		it.pos--
	} else {
		it.pos++
	}
	return it.pos >= 0 && it.pos < len(it.entries)
}

// Seek moves the iterator to the first key >= k (forward) or last key <= k (reverse).
// Entries are always in ascending order; direction only affects Next.
func (it *Iterator) Seek(k []byte) error {
	if it.err != nil {
		return it.err
	}
	if len(k) == 0 {
		if it.reverse {
			it.pos = len(it.entries) - 1
		} else {
			it.pos = 0
		}
		return nil
	}

	if it.reverse {
		// Find last entry <= k: first index where key > k, then back one.
		idx := sort.Search(len(it.entries), func(i int) bool {
			return bytes.Compare(it.entries[i].key, k) > 0
		})
		it.pos = idx - 1
	} else {
		// Find first entry >= k.
		it.pos = sort.Search(len(it.entries), func(i int) bool {
			return bytes.Compare(it.entries[i].key, k) >= 0
		})
	}
	return nil
}

// Close closes the iterator.
func (it *Iterator) Close() {
	if it.err == nil {
		it.err = context.Canceled
	}
}

// _ is a type assertion.
var _ kvtx.Iterator = (*Iterator)(nil)
