package kvtx_genji

import (
	"bytes"
	"context"
	"sync/atomic"

	kvtx_iterator "github.com/aperturerobotics/hydra/kvtx/iterator"
	gengine "github.com/genjidb/genji/engine"
)

// additional wrapping required to handle ctx cases

// Iterator implements the GenjiDB iterator interface.
type Iterator struct {
	closed    atomic.Bool
	closedErr atomic.Pointer[error]
	*kvtx_iterator.Iterator
	s *Store
}

// NewIterator constructs a new GenjiDB iterator.
func NewIterator(s *Store, opts gengine.IteratorOptions) *Iterator {
	return &Iterator{
		s: s,
		Iterator: kvtx_iterator.NewIterator(
			s.t.tx,
			bytes.Join([][]byte{s.prefixKey, {separator}}, nil),
			true,
			opts.Reverse,
		),
	}
}

// Err returns the current iterator error.
func (i *Iterator) Err() error {
	if err := i.closedErr.Load(); err != nil {
		return *err
	}
	select {
	case <-i.s.t.ctx.Done():
		i.closed.Store(true)
		err := context.Canceled
		i.closedErr.Store(&err)
		return err
	default:
	}
	if err := i.Iterator.Err(); err != nil {
		i.closed.Store(true)
		i.closedErr.Store(&err)
		return err
	}
	if i.closed.Load() {
		return context.Canceled
	}
	return nil
}

// Valid returns whether the iterator is positioned on a valid item or not.
func (i *Iterator) Valid() bool {
	if err := i.Err(); err != nil {
		return false
	}
	return i.Iterator.Valid()
}

// Item returns the current item.
func (i *Iterator) Item() gengine.Item {
	if _, err := i.Iterator.Initialize(); err != nil {
		return nil
	}
	if err := i.Err(); err != nil {
		return nil
	}
	if !i.Iterator.Valid() {
		return nil
	}
	k := i.Iterator.Key()
	return newItem(i.s, k[len(i.s.prefixKey)+1:], nil)
}

// Next advances to the next item.
func (i *Iterator) Next() {
	select {
	case <-i.s.t.ctx.Done():
		i.closed.Store(true)
		return
	default:
		_ = i.Iterator.Next()
	}
}

// Close closes the iterator.
func (i *Iterator) Close() error {
	if !i.closed.Swap(true) {
		i.Iterator.Close()
	}
	return nil
}

// Seek moves the iterator to the selected key. If the key doesn't exist, it must move to the
// next smallest key greater than k.
func (i *Iterator) Seek(k []byte) {
	if i.Err() != nil {
		return
	}
	if len(k) != 0 {
		// genjidb: prepend prefix
		k = bytes.Join([][]byte{i.s.prefixKey, k}, []byte{separator})
	}
	if err := i.Iterator.Seek(k); err != nil {
		i.closed.Store(true)
		i.closedErr.Store(&err)
		i.Iterator.Close()
	}
}

// _ is a type assertion
var _ gengine.Iterator = ((*Iterator)(nil))
