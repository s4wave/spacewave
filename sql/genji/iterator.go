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
	closed uint32
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
	select {
	case <-i.s.t.ctx.Done():
		atomic.StoreUint32(&i.closed, 1)
		return context.Canceled
	default:
	}
	if err := i.Iterator.Err(); err != nil {
		return err
	}
	if atomic.LoadUint32(&i.closed) == 1 {
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
	return newItem(i.s, k[len(i.s.prefixKey)+1:])
}

// Next advances to the next item.
func (i *Iterator) Next() {
	select {
	case <-i.s.t.ctx.Done():
		atomic.StoreUint32(&i.closed, 1)
		return
	default:
		_ = i.Iterator.Next()
	}
}

// Close closes the iterator.
func (i *Iterator) Close() error {
	i.Iterator.Close()
	atomic.StoreUint32(&i.closed, 1)
	return nil
}

// Seek moves the iterator to the selected key. If the key doesn't exist, it must move to the
// next smallest key greater than k.
func (i *Iterator) Seek(k []byte) {
	if len(k) != 0 {
		// genjidb: prepend prefix
		k = bytes.Join([][]byte{i.s.prefixKey, k}, []byte{separator})
	}
	i.Iterator.Seek(k)
}

// _ is a type assertion
var _ gengine.Iterator = ((*Iterator)(nil))
