package kvtx_genji

import (
	"bytes"
	"context"
	"errors"

	"github.com/emirpasic/gods/sets/treeset"
	gengine "github.com/genjidb/genji/engine"
)

// Iterator implements the GenjiDB iterator interface.
type Iterator struct {
	s   *Store
	err error
	rev bool
	oob bool

	// keys is a workaround to produce sorted / seekable output
	keys *treeset.Set
	ki   *treeset.Iterator
}

// NewIterator constructs a new GenjiDB iterator.
func NewIterator(s *Store, opts gengine.IteratorOptions) *Iterator {
	// TODO: Implement iterator properly in Hydra kvtx store.
	// Primary issue: Hydra Scan does not produce sorted results
	// Workaround here: scan all keys in advance to build a sorted set (slow).
	keys := treeset.NewWith(func(a, b interface{}) int {
		b1 := a.([]byte)
		b2 := b.([]byte)
		return bytes.Compare(b1, b2)
	})
	err := s.t.tx.ScanPrefix(s.prefixKey, func(key, _ []byte) error {
		if len(key) <= len(s.prefixKey)+1 {
			return nil // skip
		}

		kb := make([]byte, len(key)-len(s.prefixKey)-1)
		copy(kb, key[len(s.prefixKey)+1:])
		keys.Add(kb)
		return nil
	})
	ki := keys.Iterator()
	oob := ki.Index() >= keys.Size()
	return &Iterator{
		s:    s,
		rev:  opts.Reverse,
		oob:  oob,
		keys: keys,
		ki:   &ki,
		err:  err,
	}
}

// Seek moves the iterator to the selected key. If the key doesn't exist, it must move to the
// next smallest key greater than k.
func (i *Iterator) Seek(k []byte) {
	if i.err != nil {
		return
	}
	select {
	case <-i.s.t.ctx.Done():
		i.err = context.Canceled
		return
	default:
	}
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
		for bytes.Compare(i.ki.Value().([]byte), k) > 0 {
			if !i.ki.Prev() {
				i.oob = true
			}
		}
	}
}

// Next moves the iterator to the next item.
func (i *Iterator) Next() {
	if i.oob || i.err != nil {
		return
	}
	select {
	case <-i.s.t.ctx.Done():
		i.err = context.Canceled
		return
	default:
	}
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

// Err returns an error that invalidated iterator.
// If Err is not nil then Valid must return false.
func (i *Iterator) Err() error {
	return i.err
}

// Valid returns whether the iterator is positioned on a valid item or not.
func (i *Iterator) Valid() bool {
	return i.err == nil && !i.oob
}

// Item returns the current item.
func (i *Iterator) Item() gengine.Item {
	select {
	case <-i.s.t.ctx.Done():
		i.err = context.Canceled
	default:
	}
	if i.oob || i.err != nil {
		return nil
	}
	return newItem(i.s, i.ki.Value().([]byte))
}

// Close releases the resources associated with the iterator.
func (i *Iterator) Close() error {
	return errors.New("TODO hydra genjidb iterator Close")
}

// _ is a type assertion
var _ gengine.Iterator = ((*Iterator)(nil))
