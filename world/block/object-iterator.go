package world_block

import (
	"context"

	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/aperturerobotics/hydra/world"
)

// objectIterator implements ObjectIterator for WorldState.
type objectIterator struct {
	w        *WorldState
	ctx      context.Context
	prefix   string
	reversed bool
	iter     kvtx.Iterator
	err      error
}

// NewObjectIterator constructs a new object iterator.
func NewObjectIterator(w *WorldState, ctx context.Context, prefix string, reversed bool) *objectIterator {
	oi := &objectIterator{
		w:        w,
		ctx:      ctx,
		prefix:   prefix,
		reversed: reversed,
	}
	oi.iter = w.objTree.Iterate(ctx, []byte(objectKeyPrefix+prefix), true, reversed)
	return oi
}

// Err returns any error that has closed the iterator.
func (o *objectIterator) Err() error {
	if o.err != nil {
		return o.err
	}
	if o.iter != nil {
		return o.iter.Err()
	}
	return context.Canceled
}

// Valid returns if the iterator points to a valid entry.
func (o *objectIterator) Valid() bool {
	return o.err == nil && o.Key() != ""
}

// Key returns the current entry key, or empty string if not valid.
func (o *objectIterator) Key() string {
	if !o.iter.Valid() {
		return ""
	}
	key := string(o.iter.Key())
	if len(key) < len(objectKeyPrefix)+len(o.prefix) {
		return ""
	}
	return key[len(objectKeyPrefix):]
}

// Next advances to the next entry and returns Valid.
func (o *objectIterator) Next() bool {
	return o.iter.Next() && o.Valid()
}

// Seek moves the iterator to the first key >= the provided key (or <= in reverse mode).
func (o *objectIterator) Seek(k string) error {
	if o.err != nil {
		return o.err
	}
	if o.iter == nil {
		return context.Canceled
	}

	err := o.iter.Seek([]byte(objectKeyPrefix + k))
	if err != nil {
		o.err = err
		return err
	}

	return nil
}

// Close releases the iterator.
func (o *objectIterator) Close() {
	if o.iter != nil {
		o.iter.Close()
		o.iter = nil
	}
	o.err = context.Canceled
}

// _ is a type assertion
var _ world.ObjectIterator = ((*objectIterator)(nil))
