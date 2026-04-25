//go:build js && wasm

package store

import (
	"bytes"

	"github.com/s4wave/spacewave/db/kvtx"
)

// opfsIterator implements kvtx.Iterator over collected directory entries.
type opfsIterator struct {
	tx    *opfsTx
	keys  [][]byte
	names []string
	pos   int
	err   error
}

// Err returns any error from the iterator.
func (it *opfsIterator) Err() error {
	return it.err
}

// Valid returns true if the iterator points to a valid entry.
func (it *opfsIterator) Valid() bool {
	return it.err == nil && it.pos >= 0 && it.pos < len(it.keys)
}

// Key returns the current key.
func (it *opfsIterator) Key() []byte {
	if !it.Valid() {
		return nil
	}
	return it.keys[it.pos]
}

// Value reads and returns the current value from OPFS.
func (it *opfsIterator) Value() ([]byte, error) {
	if !it.Valid() {
		return nil, it.err
	}
	encoded := it.names[it.pos]
	dir, err := it.tx.getShardDir(encoded)
	if err != nil {
		it.err = err
		return nil, err
	}
	fh, err := dir.GetFileHandle(encoded, false)
	if err != nil {
		it.err = err
		return nil, err
	}
	data, err := fh.ReadFile()
	if err != nil {
		it.err = err
		return nil, err
	}
	return data, nil
}

// ValueCopy returns a copy of the current value.
func (it *opfsIterator) ValueCopy(dst []byte) ([]byte, error) {
	data, err := it.Value()
	if err != nil {
		return nil, err
	}
	if cap(dst) >= len(data) {
		dst = dst[:len(data)]
	} else {
		dst = make([]byte, len(data))
	}
	copy(dst, data)
	return dst, nil
}

// Next advances to the next entry.
func (it *opfsIterator) Next() bool {
	if it.err != nil {
		return false
	}
	it.pos++
	return it.pos < len(it.keys)
}

// Seek moves the iterator to the given key or the next key after it.
func (it *opfsIterator) Seek(k []byte) error {
	for i, key := range it.keys {
		if bytes.Compare(key, k) >= 0 {
			it.pos = i
			return nil
		}
	}
	it.pos = len(it.keys)
	return nil
}

// Close closes the iterator.
func (it *opfsIterator) Close() {
	it.pos = len(it.keys)
}

// _ is a type assertion
var _ kvtx.Iterator = (*opfsIterator)(nil)
