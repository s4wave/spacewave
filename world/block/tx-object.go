package world_block

import (
	"context"

	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/tx"
	"github.com/aperturerobotics/hydra/world"
)

// CreateObject creates a object with a key and initial root ref.
// Returns ErrObjectExists if the object already exists.
func (t *Tx) CreateObject(ctx context.Context, key string, rootRef *bucket.ObjectRef) (world.ObjectState, error) {
	unlock, err := t.rmtx.Lock(ctx, true)
	if err != nil {
		return nil, err
	}
	defer unlock()

	if t.discarded {
		return nil, tx.ErrDiscarded
	}

	cobj, err := t.state.CreateObject(ctx, key, rootRef)
	if err != nil || cobj == nil {
		return nil, err
	}
	return NewTxObjectState(t, key, cobj), nil
}

// GetObject looks up an object by key.
// Returns nil, false if not found.
func (t *Tx) GetObject(ctx context.Context, key string) (world.ObjectState, bool, error) {
	unlock, err := t.rmtx.Lock(ctx, false)
	if err != nil {
		return nil, false, err
	}
	defer unlock()

	if t.discarded {
		return nil, false, tx.ErrDiscarded
	}

	cobj, ok, err := t.state.GetObject(ctx, key)
	if err != nil || !ok || cobj == nil {
		return nil, ok, err
	}
	return NewTxObjectState(t, key, cobj), true, nil
}

// DeleteObject deletes an object and associated graph quads by ID.
// Calls DeleteGraphObject internally.
// Returns false, nil if not found.
func (t *Tx) DeleteObject(ctx context.Context, key string) (bool, error) {
	unlock, err := t.rmtx.Lock(ctx, true)
	if err != nil {
		return false, err
	}
	defer unlock()

	return t.state.DeleteObject(ctx, key)
}

// _ is a type assertion
var _ world.WorldStateObject = ((*Tx)(nil))
