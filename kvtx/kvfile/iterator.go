package kvtx_kvfile

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/aperturerobotics/hydra/kvtx"
)

// Iterator iterates over a kvfile store.
type Iterator struct {
	rdr     *kvfile.Reader
	prefix  []byte
	reverse bool

	err error
	oob bool
	end bool // end indicates Next() is necessary

	idx int

	// may be nil
	entry *kvfile.IndexEntry
	val   *[]byte
}

// NewIterator constructs a new iterator.
func NewIterator(rdr *kvfile.Reader, prefix []byte, reverse bool) *Iterator {
	return &Iterator{rdr: rdr, prefix: prefix, reverse: reverse, end: true}
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
	return i.err == nil && !i.oob && !i.end && i.idx >= 0 && i.idx < int(i.rdr.Size())
}

// Key returns the current entry key, or nil if not valid.
func (i *Iterator) Key() []byte {
	if !i.Valid() {
		return nil
	}
	return i.entry.GetKey()
}

// Value returns the current entry value, or nil if not valid.
//
// May cache the value between calls, copy if modifying.
func (i *Iterator) Value() ([]byte, error) {
	if !i.Valid() {
		return nil, i.Err()
	}
	if i.val != nil {
		return *i.val, nil
	}
	if i.entry == nil {
		entry, err := i.rdr.ReadIndexEntry(uint64(i.idx))
		if err != nil {
			return nil, err
		}
		i.entry = entry
	}
	val, err := i.rdr.GetWithEntry(i.entry, i.idx)
	if err != nil {
		return nil, err
	}
	if len(val) != 0 {
		i.val = &val
	}
	return val, nil
}

// ValueCopy copies the key to the given byte slice and returns it.
// If the slice is not big enough (cap), it must create a new one and return it.
// May use the value cached from Value() call as the source of the data.
// May return nil if !Valid().
func (i *Iterator) ValueCopy(bt []byte) ([]byte, error) {
	val, err := i.Value()
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
	size := i.rdr.Size()
	if size == 0 {
		i.oob = true
		return false
	}
	i.entry, i.val = nil, nil
	if i.end {
		i.end = false
		// use binary search to find first or last key w prefix
		// search for last key with prefix if reverse
		if len(i.prefix) != 0 {
			idxEntry, idx, err := i.rdr.SearchIndexEntryWithPrefix(i.prefix, i.reverse)
			if err != nil {
				i.err = err
			} else if idxEntry != nil {
				i.idx, i.entry = idx, idxEntry
			} else {
				// no keys w/ the prefix exist.
				i.oob, i.idx = true, 0
			}
		} else {
			if i.reverse {
				i.idx = int(size) - 1
				i.entry, i.err = i.rdr.ReadIndexEntry(uint64(i.idx))
			} else {
				i.idx = 0
				i.entry, i.err = i.rdr.ReadIndexEntry(uint64(i.idx))
			}
		}
	} else {
		if i.reverse {
			i.idx--
			i.oob = i.idx < 0
		} else {
			i.idx++
			i.oob = i.idx >= int(size)
		}
		if !i.oob {
			i.entry, i.err = i.rdr.ReadIndexEntry(uint64(i.idx))
			i.oob = i.entry == nil
		}
		if i.err == nil && !i.oob && i.entry != nil && len(i.prefix) != 0 {
			if !bytes.HasPrefix(i.entry.GetKey(), i.prefix) {
				i.oob = true
			}
		}
	}
	return i.Valid()
}

// Seek moves the iterator to the selected key or the next key after the key if not found.
// Pass nil to seek to the beginning (or end if reversed).
func (i *Iterator) Seek(k []byte) error {
	i.entry, i.val, i.err = nil, nil, nil
	if len(k) == 0 {
		i.end = true
		_ = i.Next()
		return i.err
	}

	// search for the key.
	i.entry, i.idx, i.err = i.rdr.SearchIndexEntryWithKey(k)
	i.oob = i.entry == nil && i.err == nil
	return nil
}

// Close closes the iterator.
func (i *Iterator) Close() {
	i.oob, i.val, i.entry = true, nil, nil
	if i.err == nil {
		i.err = context.Canceled
	}
}

// _ is a type assertion
var _ kvtx.Iterator = ((*Iterator)(nil))
