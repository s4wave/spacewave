//go:build !js && !wasip1

package store_kvtx_bolt

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/bbolt"
	"github.com/aperturerobotics/hydra/kvtx"
)

// Iterator iterates over a bbolt cursor.
type Iterator struct {
	bkt     *bbolt.Cursor
	prefix  []byte
	reverse bool

	err error
	oob bool
	end bool // end indicates Next() is necessary

	key, val []byte
}

// NewIterator constructs a new bbolt cursor iterator.
//
// Note: additional special care is taken to ensure the prefix is respected.
func NewIterator(bkt *bbolt.Cursor, prefix []byte, sort, reverse bool) *Iterator {
	_ = sort // always sorted in Bolt
	return &Iterator{bkt: bkt, prefix: prefix, reverse: reverse, end: true}
}

// Err returns any error that has closed the iterator.
// May return context.Canceled if closed.
func (i *Iterator) Err() error {
	return i.err
}

// Valid returns if the iterator points to a valid entry.
//
// If err is set, returns false.
func (i *Iterator) Valid() bool {
	return i.err == nil && !i.oob && !i.end
}

// Key returns the current entry key, or nil if not valid.
func (i *Iterator) Key() []byte {
	if !i.Valid() {
		return nil
	}
	return i.key
}

// Value returns the current entry value, or nil if not valid.
//
// May cache the value between calls, copy if modifying.
func (i *Iterator) Value() ([]byte, error) {
	if !i.Valid() {
		return nil, i.Err()
	}
	return i.val, nil
}

// ValueCopy copies the key to the given byte slice and returns it.
// If the slice is not big enough (cap), it must create a new one and return it.
// May use the value cached from Value() call as the source of the data.
// May return nil if !Valid().
func (i *Iterator) ValueCopy(bt []byte) ([]byte, error) {
	if err := i.Err(); err != nil {
		return nil, err
	}
	if !i.Valid() {
		return nil, nil
	}
	return append(bt[:0], i.val...), nil
}

// Next advances to the next entry and returns Valid.
func (i *Iterator) Next() bool {
	if err := i.Err(); err != nil {
		return false
	}
	if i.end {
		if i.reverse {
			i.key, i.val = i.bkt.Last()
		} else {
			i.key, i.val = i.bkt.First()
		}
		i.end = false
	} else {
		if i.reverse {
			i.key, i.val = i.bkt.Prev()
		} else {
			i.key, i.val = i.bkt.Next()
		}
	}
	i.oob = len(i.key) == 0
	i.skipPrefixMismatch()
	return !i.oob
}

// Seek moves the iterator to the selected key, or the next key after the key.
// Pass nil to seek to the beginning (or end if reversed).
func (i *Iterator) Seek(k []byte) error {
	if err := i.Err(); err != nil {
		return err
	}
	i.key, i.val, i.end = nil, nil, false

	if i.reverse {
		// In reverse mode:
		// 1. If k is nil/empty, seek to last key
		// 2. Otherwise seek to k and move back one if we land after k
		if len(k) == 0 {
			i.key, i.val = i.bkt.Last()
		} else {
			i.key, i.val = i.bkt.Seek(k)
			if i.reverse && bytes.Compare(i.key, k) > 0 {
				i.key, i.val = i.bkt.Prev()
			}
		}
	} else {
		if len(k) == 0 {
			i.key, i.val = i.bkt.First()
		} else {
			i.key, i.val = i.bkt.Seek(k)
		}
	}

	i.oob = len(i.key) == 0
	i.skipPrefixMismatch()
	return nil
}

// skipPrefixMismatch skips any keys that do not match the prefix.
func (i *Iterator) skipPrefixMismatch() {
	if i.oob || len(i.prefix) == 0 {
		return
	}
	for len(i.key) != 0 && !bytes.HasPrefix(i.key, i.prefix) {
		if i.reverse {
			i.key, i.val = i.bkt.Prev()
		} else {
			i.key, i.val = i.bkt.Next()
		}
		// out of bounds or prefix seen
		if len(i.key) == 0 {
			break
		}
	}
	i.oob = len(i.key) == 0
}

// Close closes the iterator.
func (i *Iterator) Close() {
	i.oob = true
	i.key = nil
	i.val = nil
	if i.err == nil {
		i.err = context.Canceled
	}
}

// _ is a type assertion
var _ kvtx.Iterator = ((*Iterator)(nil))
