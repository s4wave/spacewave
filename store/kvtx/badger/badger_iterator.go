package store_kvtx_badger

import (
	"context"

	"github.com/aperturerobotics/hydra/kvtx"
	bdb "github.com/dgraph-io/badger/v4"
)

// Iterator iterates over a badger bucket.
type Iterator struct {
	it  *bdb.Iterator
	err error
	rel func()

	key, value []byte
}

// NewIterator constructs a new iterator.
func NewIterator(it *bdb.Iterator, rel func()) *Iterator {
	return &Iterator{it: it, rel: rel}
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
	return i.err == nil && i.it.Valid()
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
	if err := i.Err(); err != nil {
		return nil, err
	}
	if !i.Valid() {
		return nil, nil
	}
	val, err := i.Value() // call ValueCopy once
	if err != nil {
		return nil, err
	}
	return append(bt[:0], val...), nil
}

// Next advances to the next entry and returns Valid.
func (i *Iterator) Next() bool {
	if err := i.Err(); err != nil {
		return false
	}
	i.it.Next()
	return i.it.Valid()
}

// Seek moves the iterator to the selected key, or the next key after the key.
// Pass nil to seek to the beginning (or end if reversed).
func (i *Iterator) Seek(k []byte) error {
	if err := i.Err(); err != nil {
		return err
	}
	i.it.Seek(k)
	return nil
}

// Close closes the iterator.
func (i *Iterator) Close() {
	i.it.Close()
	i.key = nil
	i.value = nil
	if i.err == nil {
		i.err = context.Canceled
	}
	if r := i.rel; r != nil {
		r()
	}
}

// _ is a type assertion
var _ kvtx.Iterator = ((*Iterator)(nil))
