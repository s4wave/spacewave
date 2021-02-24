package kvtx_iterator

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/emirpasic/gods/sets/treeset"
)

// Ops are tx ops.
type Ops interface {
	// Get returns values for a key.
	Get(key []byte) (data []byte, found bool, err error)
	// ScanPrefixKeys iterates over keys only with a prefix.
	ScanPrefixKeys(prefix []byte, cb func(key []byte) error) error
}

// Iterator implements the KVTX Iterator interface by scanning all keys into an
// in-memory tree set. This implements (fills) the Iterator interface for stores
// that do not support iteration.
//
// Note: iterator is tested in store/kvtx/inmem.
type Iterator struct {
	s   Ops
	err error
	// TODO: support unsorted iteration
	// sort   bool
	rev    bool
	oob    bool
	prefix []byte
	val    []byte

	// keys is a workaround to produce sorted / seekable output
	// keys is nil until Initialize is called
	keys *treeset.Set
	ki   *treeset.Iterator
}

// NewIterator constructs a new iterator. Initial key fetch is deferred to the
// first Next() call.
func NewIterator(s Ops, prefix []byte, sort, reverse bool) *Iterator {
	return &Iterator{
		s:   s,
		rev: reverse,
		// TODO sort:   sort,
		prefix: prefix,
	}
}

// Initialize intializes the iterator, fetching all keys into memory.
func (i *Iterator) Initialize() (skipNext bool, err error) {
	if i.keys != nil || i.err != nil {
		return false, i.err
	}

	// Primary issue: Hydra Scan does not produce sorted results
	// Workaround here: scan all keys in advance to build a sorted set (slow).
	keys := treeset.NewWith(func(a, b interface{}) int {
		b1 := a.([]byte)
		b2 := b.([]byte)
		return bytes.Compare(b1, b2)
	})
	err = i.s.ScanPrefixKeys(i.prefix, func(key []byte) error {
		kb := make([]byte, len(key))
		copy(kb, key)
		keys.Add(kb)
		return nil
	})
	i.keys = keys
	i.err = err
	if err != nil {
		return false, err
	}
	ki := keys.Iterator()
	i.ki = &ki

	// TODO: this means that Next() is now unnecessary.
	// return some bool to indicate skipping Next()
	if i.rev {
		i.oob = !ki.Last()
	} else {
		i.oob = !ki.First()
	}
	return true, nil
}

// Seek moves the iterator to the selected key. If the key doesn't exist, it must move to the
// next smallest key greater than k.
func (i *Iterator) Seek(k []byte) {
	if _, err := i.Initialize(); err != nil {
		return
	}
	i.val = nil
	if len(k) == 0 {
		if i.rev {
			i.oob = !i.ki.Last()
		} else {
			i.oob = !i.ki.First()
		}
		return
	}

	i.ki.Begin()
	for i.ki.Next() {
		// k <= k_i
		if bytes.Compare(k, i.ki.Value().([]byte)) <= 0 {
			break
		}
	}
	i.oob = i.ki.Index() >= i.keys.Size()

	if i.rev {
		if i.oob {
			if !i.ki.Last() {
				return
			}
			i.oob = false
		}
		// if reversed, iterate backwards while key > target
		// this will find the key that is <= target
		for bytes.Compare(i.ki.Value().([]byte), k) > 0 {
			if !i.ki.Prev() {
				i.oob = true
				break
			}
		}
	}
}

// Next moves the iterator to the next item.
func (i *Iterator) Next() bool {
	skipNext, err := i.Initialize()
	if i.oob || err != nil {
		return false
	}
	i.val = nil
	if !skipNext {
		if i.rev {
			if i.ki.Index() == 0 {
				i.oob = true
			} else {
				i.ki.Prev()
			}
		} else {
			if i.ki.Index()+1 >= i.keys.Size() {
				// last index
				i.oob = true
			} else {
				i.ki.Next()
			}
		}
	}
	return i.Valid()
}

// Err returns an error that invalidated iterator.
// If Err is not nil then Valid must return false.
func (i *Iterator) Err() error {
	return i.err
}

// Valid returns whether the iterator is positioned on a valid item or not.
func (i *Iterator) Valid() bool {
	return i.err == nil && !i.oob && i.keys != nil
}

// Key returns the current key.
func (i *Iterator) Key() []byte {
	if _, err := i.Initialize(); err != nil {
		return nil
	}
	if i.oob || i.ki == nil {
		return nil
	}
	b, ok := i.ki.Value().([]byte)
	if ok {
		return b
	}
	return nil
}

// Value returns the current value.
func (i *Iterator) Value() []byte {
	if _, err := i.Initialize(); err != nil {
		return nil
	}
	if i.oob || i.ki == nil {
		return nil
	}
	if len(i.val) != 0 {
		return i.val
	}
	v, _ := i.ValueCopy(nil) // sets i.val internally
	return v
}

// ValueCopy copies the key to the given byte slice and returns it.
// If the slice is not big enough (cap), it must create a new one and return it.
// Always returns a new copy (does not cache between calls).
func (i *Iterator) ValueCopy(bt []byte) ([]byte, error) {
	if _, err := i.Initialize(); err != nil {
		return nil, err
	}
	if i.oob || i.ki == nil {
		return nil, nil
	}
	var err error
	var val []byte
	if len(i.val) != 0 {
		val = i.val
	} else {
		var found bool
		val, found, err = i.s.Get(i.Key())
		if err != nil {
			return nil, err
		}
		if !found {
			val = nil
		}
		i.val = val
	}
	if len(val) == 0 {
		return bt[:0], nil
	}
	return append(bt[:0], val...), nil
}

// Close releases the resources associated with the iterator.
func (i *Iterator) Close() {
	i.oob = true
	i.val = nil
	if i.err == nil {
		i.err = context.Canceled
	}
}

// _ is a type assertion
var _ kvtx.Iterator = ((*Iterator)(nil))
