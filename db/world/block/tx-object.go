package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
)

// GetObject looks up an object by key.
// Returns nil, false if not found.
func (t *Tx) GetObject(ctx context.Context, key string) (world.ObjectState, bool, error) {
	// Load the object under the read lock, releasing a miss there.
	var cobj world.ObjectState
	var ok bool
	err := t.locked(ctx, false, func() error {
		var err error
		cobj, ok, err = t.state.GetObject(ctx, key)
		if err != nil || !ok || cobj == nil {
			world.ReleaseObjectState(cobj)
			cobj = nil
		}
		return err
	})
	if cobj == nil {
		return nil, ok, err
	}
	return NewTxObjectState(t, key, cobj), true, nil
}

// IterateObjects returns an iterator with the given object key prefix.
// The prefix is NOT clipped from the output keys.
// Keys are returned in sorted order.
// Must call Next() or Seek() before valid.
// Call Close when done with the iterator.
// Any init errors will be available via the iterator's Err() method.
func (t *Tx) IterateObjects(ctx context.Context, prefix string, reversed bool) world.ObjectIterator {
	// Open the state iterator under the read lock. Each step of the iterator
	// takes the read lock again, so no step may run while this call holds it.
	var it world.ObjectIterator
	err := t.lockedLive(ctx, func() error {
		it = t.state.IterateObjects(ctx, prefix, reversed)
		return nil
	})
	if err != nil {
		return &lockedObjectIterator{err: err}
	}
	run := func(fn func(world.ObjectIterator) error) error {
		return t.lockedLive(ctx, func() error { return fn(it) })
	}
	return &lockedObjectIterator{run: run, held: it}
}

// CreateObject creates a object with a key and initial root ref.
// Returns ErrObjectExists if the object already exists.
func (t *Tx) CreateObject(ctx context.Context, key string, rootRef *bucket.ObjectRef) (world.ObjectState, error) {
	return lockedValue(ctx, t, true, func() (world.ObjectState, error) {
		cobj, err := t.state.CreateObject(ctx, key, rootRef)
		if err != nil || cobj == nil {
			world.ReleaseObjectState(cobj)
			return nil, err
		}
		return NewTxObjectState(t, key, cobj), nil
	})
}

// RenameObject renames an object key and updates associated graph quads.
func (t *Tx) RenameObject(ctx context.Context, oldKey, newKey string, descendants bool) (world.ObjectState, error) {
	return lockedValue(ctx, t, true, func() (world.ObjectState, error) {
		cobj, err := t.state.RenameObject(ctx, oldKey, newKey, descendants)
		if err != nil || cobj == nil {
			world.ReleaseObjectState(cobj)
			return nil, err
		}
		return NewTxObjectState(t, newKey, cobj), nil
	})
}

// DeleteObject deletes an object and associated graph quads by ID.
// Calls DeleteGraphObject internally.
// Returns false, nil if not found.
func (t *Tx) DeleteObject(ctx context.Context, key string) (bool, error) {
	return lockedValue(ctx, t, true, func() (bool, error) {
		return t.state.DeleteObject(ctx, key)
	})
}

// PurgeObject removes the object entry and its graph quads without reading
// the object's record. Returns false, nil if not found.
func (t *Tx) PurgeObject(ctx context.Context, key string) (bool, error) {
	return lockedValue(ctx, t, true, func() (bool, error) {
		return t.state.PurgeObject(ctx, key)
	})
}

// HasObject reports whether an object exists at key. It takes the write lock
// because a miss may populate the underlying state's transaction-local object
// memo, which the single-threaded state does not guard on its own.
func (t *Tx) HasObject(ctx context.Context, key string) (bool, error) {
	return lockedValue(ctx, t, true, func() (bool, error) {
		return t.state.HasObject(ctx, key)
	})
}

// _ is a type assertion
var _ world.WorldStateObject = (*Tx)(nil)
