package world_block_tx

import (
	"context"

	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/world"
)

// ObjectIterator implements ObjectIterator for WorldState.
type ObjectIterator struct {
	// w is the world state
	w *WorldState
	// ctx is the context
	ctx context.Context
	// prefix is the prefix to filter by
	prefix string
	// reversed indicates if iteration is reversed
	reversed bool

	// err is any error that occurred
	err error
	// currKey is the current key if valid
	currKey string
	// valid indicates if the iterator is valid
	valid bool
	// iter is the underlying iterator
	iter world.ObjectIterator
}

// NewObjectIterator constructs a new object iterator.
func NewObjectIterator(
	w *WorldState,
	ctx context.Context,
	prefix string,
	reversed bool,
) *ObjectIterator {
	return &ObjectIterator{
		w:        w,
		ctx:      ctx,
		prefix:   prefix,
		reversed: reversed,
	}
}

// Err returns any error that has closed the iterator.
func (o *ObjectIterator) Err() error {
	return o.err
}

// Valid returns if the iterator points to a valid entry.
func (o *ObjectIterator) Valid() bool {
	return o.err == nil && o.valid
}

// Key returns the current entry key, or empty string if not valid.
func (o *ObjectIterator) Key() string {
	if !o.Valid() {
		return ""
	}
	return o.currKey
}

// Next advances to the next entry and returns Valid.
func (o *ObjectIterator) Next() bool {
	if o.err != nil {
		return false
	}

	o.w.mtx.Lock()
	defer o.w.mtx.Unlock()

	if o.w.discarded {
		o.err = kvtx.ErrDiscarded
		o.valid = false
		return false
	}

	// Initialize iterator if not already done
	if o.iter == nil {
		o.iter = o.w.world.IterateObjects(o.ctx, o.prefix, o.reversed)
		if o.iter == nil {
			o.valid = false
			return false
		}
	}

	if !o.iter.Next() {
		o.err = o.iter.Err()
		o.valid = false
		return false
	}

	if !o.iter.Valid() {
		o.err = o.iter.Err()
		o.valid = false
		return false
	}

	o.currKey = o.iter.Key()
	o.valid = true
	return true
}

// Seek moves the iterator to the first key >= the provided key (or <= in reverse mode).
func (o *ObjectIterator) Seek(k string) error {
	if o.err != nil {
		return o.err
	}

	o.w.mtx.Lock()
	defer o.w.mtx.Unlock()

	if o.w.discarded {
		o.err = kvtx.ErrDiscarded
		o.valid = false
		return o.err
	}

	// Initialize iterator if not already done
	if o.iter == nil {
		o.iter = o.w.world.IterateObjects(o.ctx, o.prefix, o.reversed)
		if o.iter == nil {
			o.valid = false
			return nil
		}
	}

	if err := o.iter.Seek(k); err != nil {
		o.err = err
		o.valid = false
		return err
	}

	if !o.iter.Valid() {
		o.err = o.iter.Err()
		o.valid = false
		return o.err
	}

	o.currKey = o.iter.Key()
	o.valid = true
	return nil
}

// Close releases the iterator.
func (o *ObjectIterator) Close() {
	if o.iter != nil {
		o.iter.Close()
		o.iter = nil
	}
	o.valid = false
	o.err = context.Canceled
}

// _ is a type assertion
var _ world.ObjectIterator = ((*ObjectIterator)(nil))
