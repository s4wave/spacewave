package kvtx_iterator

import (
	"bytes"
	"context"

	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/tidwall/btree"
)

// TODO: avoid using this and remove it wherever possible.

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

	// Always use forward comparison for the tree
	less := func(a, b []byte) bool { return bytes.Compare(a, b) < 0 }
	keys := btree.NewBTreeGOptions(less, btree.Options{
		NoLocks: true,
	})

	// Build items list with prefix filtering
	err = i.s.ScanPrefixKeys(i.ctx, i.prefix, func(key []byte) error {
		if len(i.prefix) != 0 && !bytes.HasPrefix(key, i.prefix) {
			return nil
		}
		keys.Set(bytes.Clone(key))
		return nil
	})
	i.keys = keys
	i.err = err
	if err != nil {
		return false, err
	}
	i.ki = keys.Iter()

	// Start before first/after last item depending on direction
	if i.rev {
		i.oob = !i.ki.Last()
	} else {
		i.oob = !i.ki.First()
	}
	return true, nil
}

// Seek moves the iterator to the first key >= the provided key (or <= in reverse mode).
// Pass nil to seek to the beginning (or end if reversed).
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

	// Binary search for the key
	valid := i.ki.Seek(k)

	if i.rev {
		// In reverse mode:
		// If we found an exact match, stay there
		// If we didn't find an exact match and landed on a greater key, move back one
		if valid {
			if bytes.Compare(i.ki.Item(), k) > 0 {
				valid = i.ki.Prev()
			}
		} else {
			// If we went past the end, move to the last item
			valid = i.ki.Last()
		}
	}

	i.oob = !valid

	// Check if the key matches our prefix constraint
	if valid && len(i.prefix) != 0 {
		key := i.ki.Item()
		if !bytes.HasPrefix(key, i.prefix) {
			i.oob = true
		}
	}

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
		var valid bool
		if i.rev {
			valid = i.ki.Prev()
		} else {
			valid = i.ki.Next()
		}
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
	if i.err == nil {
		i.err = context.Canceled
	}
	i.oob = true
	i.val = nil
	i.keys = nil
}

// _ is a type assertion
var _ kvtx.Iterator = ((*Iterator)(nil))
