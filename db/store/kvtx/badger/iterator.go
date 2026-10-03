package store_kvtx_badger

import (
	"bytes"
	"context"

	bdb "github.com/dgraph-io/badger/v4"
	"github.com/s4wave/spacewave/db/kvtx"
)

// Iterator iterates over a badger bucket.
type Iterator struct {
	it     *bdb.Iterator
	err    error
	rev    bool
	prefix []byte
	rel    func()
	// positioned is set after the first Seek.
	positioned bool

	key, value []byte
}

// NewIterator constructs a new iterator.
func NewIterator(it *bdb.Iterator, rev bool, prefix []byte, rel func()) *Iterator {
	return &Iterator{it: it, rev: rev, prefix: prefix, rel: rel}
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
	return i.err == nil && i.it.ValidForPrefix(i.prefix)
}

// Key returns the current entry key, or nil if not valid.
func (i *Iterator) Key() []byte {
	if !i.Valid() {
		return nil
	}
	if len(i.key) == 0 {
		i.key = i.it.Item().KeyCopy(nil)
	}
	return i.key
}

// Value returns the current entry value, or nil if not valid.
//
// May cache the value between calls, copy if modifying.
func (i *Iterator) Value() ([]byte, error) {
	if err := i.Err(); err != nil {
		return nil, err
	}
	if !i.Valid() {
		return nil, nil
	}
	if len(i.value) == 0 {
		var err error
		i.value, err = i.it.Item().ValueCopy(nil)
		if err != nil {
			i.err = err
			i.value = nil
		}
	}
	return i.value, nil
}

// ValueCopy copies the key to the given byte slice and returns it.
// If the slice is not big enough (cap), it must create a new one and return it.
// May use the value cached from Value() call as the source of the data.
// May return nil if !Valid().
func (i *Iterator) ValueCopy(bt []byte) ([]byte, error) {
	// Require a valid Badger entry before copying its cached value.
	if err := i.Err(); err != nil {
		return nil, err
	}
	if !i.Valid() {
		return nil, nil
	}

	// Read the Badger value and copy it into the caller buffer.
	val, err := i.Value() // call ValueCopy once
	if err != nil {
		return nil, err
	}
	return append(bt[:0], val...), nil
}

// Next advances to the next entry and returns Valid.
//
// Calling Next before Seek positions the iterator at the first entry.
func (i *Iterator) Next() bool {
	// Position a fresh Badger iterator or stop an invalid traversal.
	if err := i.Err(); err != nil {
		return false
	}
	if !i.positioned {
		if err := i.Seek(nil); err != nil {
			return false
		}
		return i.Valid()
	}
	if !i.Valid() {
		return false
	}

	// Advance the Badger iterator and invalidate its cached entry.
	i.key, i.value = nil, nil
	i.it.Next()
	return i.Valid()
}

// Seek moves the iterator to the first key >= k, or <= k if reversed.
// Pass nil to seek to the beginning (or end if reversed).
func (i *Iterator) Seek(k []byte) error {
	// Reset the Badger entry cache and handle seeks to the prefix boundary.
	if err := i.Err(); err != nil {
		return err
	}
	i.key, i.value = nil, nil
	i.positioned = true
	if len(k) == 0 {
		if !i.rev {
			i.it.Rewind()
		} else {
			i.seekPrefixEnd()
		}
		return nil
	}

	// Keep seeks beyond the Badger prefix range at its traversal boundary.
	if len(i.prefix) != 0 && !bytes.HasPrefix(k, i.prefix) && bytes.Compare(k, i.prefix) > 0 {
		// k is past the prefix range.
		if i.rev {
			i.seekPrefixEnd()
		} else if incPrefix, ok := kvtx.PrefixSuccessor(i.prefix); ok {
			i.it.Seek(incPrefix)
		} else {
			i.it.Seek(i.prefix)
		}
		return nil
	}
	if len(i.prefix) != 0 && !i.rev && bytes.Compare(k, i.prefix) < 0 {
		// k is before the prefix range: start at the first prefixed key.
		k = i.prefix
	}

	// A reverse seek before the prefix range lands outside it and is invalid.
	i.it.Seek(k)
	return nil
}

// Close closes the iterator.
func (i *Iterator) Close() {
	// Close the Badger iterator and invalidate its cached entry.
	i.it.Close()
	i.key = nil
	i.value = nil
	if i.err == nil {
		i.err = context.Canceled
	}

	// Remove the closed iterator from its transaction tracking set.
	if r := i.rel; r != nil {
		r()
	}
}

// seekPrefixEnd positions a reverse iterator at the last key with the prefix.
func (i *Iterator) seekPrefixEnd() {
	incPrefix, ok := kvtx.PrefixSuccessor(i.prefix)
	if !ok {
		i.it.Rewind()
		return
	}
	// Reverse Seek lands on the last key <= incPrefix: step off incPrefix.
	i.it.Seek(incPrefix)
	if i.it.ValidForPrefix(incPrefix) {
		i.it.Next()
	}
}

// _ is a type assertion
var _ kvtx.Iterator = (*Iterator)(nil)
