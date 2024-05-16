package kvtx_iterator

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/tidwall/btree"
)

// TODO TODO

// Ops are tx ops.
type Ops interface {
	// Get returns values for a key.
	Get(ctx context.Context, key []byte) (data []byte, found bool, err error)
	// ScanPrefixKeys iterates over keys only with a prefix.
	ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error
}

// Iterator implements the KVTX Iterator interface by scanning all keys into an
// in-memory tree set. This implements (fills) the Iterator interface for stores
// that do not support iteration.
//
// Note: iterator is tested in store/kvtx/inmem.
type Iterator struct {
	ctx context.Context
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
	keys *btree.BTreeG[[]byte]
	ki   btree.IterG[[]byte]
}

// NewIterator constructs a new iterator. Initial key fetch is deferred to the
// first Next() call.
func NewIterator(ctx context.Context, s Ops, prefix []byte, sort, reverse bool) *Iterator {
	return &Iterator{
		ctx: ctx,
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
	// Workaround here: scan all keys in advance to build a sorted set (slow, but works).
	less := func(a, b []byte) bool { return bytes.Compare(a, b) < 0 }
	if i.rev {
		less = func(a, b []byte) bool { return bytes.Compare(a, b) > 0 }
	}
	keys := btree.NewBTreeGOptions(less, btree.Options{
		NoLocks: true,
	})
	err = i.s.ScanPrefixKeys(i.ctx, i.prefix, func(key []byte) error {
		keys.Set(key)
		return nil
	})
	i.keys = keys
	i.err = err
	if err != nil {
		return false, err
	}
	i.ki = keys.Iter()

	// TODO: this means that Next() is now unnecessary.
	// return some bool to indicate skipping Next()
	i.oob = !i.ki.First()
	return true, nil
}

// Seek moves the iterator to the selected key. If the key doesn't exist, it must move to the
// next smallest key greater than k.
func (i *Iterator) Seek(k []byte) error {
	if _, err := i.Initialize(); err != nil {
		return err
	}
	i.val = nil
	if len(k) == 0 {
		if i.rev {
			i.oob = !i.ki.Last()
		} else {
			i.oob = !i.ki.First()
		}
		return nil
	}

	valid := i.ki.Seek(k)
	i.oob = !valid

	return nil
}

// Next moves the iterator to the next item.
func (i *Iterator) Next() bool {
	skipNext, err := i.Initialize()
	if i.oob || err != nil {
		return false
	}
	i.val = nil
	if !skipNext {
		valid := i.ki.Next()
		i.oob = !valid
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
	if i.oob {
		return nil
	}
	return i.ki.Item()
}

// Value returns the current value.
func (i *Iterator) Value() ([]byte, error) {
	if _, err := i.Initialize(); err != nil {
		return nil, err
	}
	if i.oob {
		return nil, nil
	}
	if len(i.val) != 0 {
		return i.val, nil
	}
	v, _ := i.ValueCopy(nil) // sets i.val internally
	return v, nil
}

// ValueCopy copies the key to the given byte slice and returns it.
// If the slice is not big enough (cap), it must create a new one and return it.
// Always returns a new copy (does not cache between calls).
func (i *Iterator) ValueCopy(bt []byte) ([]byte, error) {
	if _, err := i.Initialize(); err != nil {
		return nil, err
	}
	if i.oob {
		return nil, nil
	}
	var err error
	var val []byte
	if len(i.val) != 0 {
		val = i.val
	} else {
		var found bool
		val, found, err = i.s.Get(i.ctx, i.Key())
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
