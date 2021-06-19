package world_block

import (
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/tx"
	"github.com/aperturerobotics/hydra/world"
)

// CreateObject creates an empty object with a key.
// Returns ErrObjectExists if the object already exists.
func (t *Tx) CreateObject(key string, rootRef *bucket.ObjectRef) (world.ObjectState, error) {
	t.rmtx.Lock()
	defer t.rmtx.Unlock()

	if t.discarded {
		return nil, tx.ErrDiscarded
	}

	cobj, err := t.state.CreateObject(key, rootRef)
	if err != nil {
		return nil, err
	}
	if cobj == nil {
		// not supposed to happen
		return nil, nil
	}
	return NewTxObjectState(t, key, cobj), nil
}

// GetObject looks up an object by key.
// Returns nil, false if not found.
func (t *Tx) GetObject(key string) (world.ObjectState, bool, error) {
	t.rmtx.RLock()
	defer t.rmtx.RUnlock()

	if t.discarded {
		return nil, false, tx.ErrDiscarded
	}

	cobj, ok, err := t.state.GetObject(key)
	if err != nil || !ok || cobj == nil {
		return nil, ok, err
	}
	return NewTxObjectState(t, key, cobj), true, nil
}

// DeleteObject deletes an object and associated graph quads by ID.
// Calls DeleteGraphObject internally.
// Returns false, nil if not found.
func (t *Tx) DeleteObject(key string) (bool, error) {
	t.rmtx.Lock()
	defer t.rmtx.Unlock()

	if t.discarded {
		return false, tx.ErrDiscarded
	}

	return t.state.DeleteObject(key)
}

// _ is a type assertion
var _ world.WorldStateObject = ((*Tx)(nil))
