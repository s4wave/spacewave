package world_block

import (
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/tx"
	"github.com/aperturerobotics/hydra/world"
)

// EngineTxObjectState is an ObjectState handle attached to a EngineTx.
type EngineTxObjectState struct {
	t   *EngineTx
	key string
}

// newEngineTxObjectState constructs a new EngineTx ObjectState object.
func newEngineTxObjectState(t *EngineTx, key string) *EngineTxObjectState {
	return &EngineTxObjectState{t: t, key: key}
}

// GetRootRef returns the root reference of the object.
func (t *EngineTxObjectState) GetRootRef() (*bucket.ObjectRef, error) {
	var rref *bucket.ObjectRef
	err := t.t.performOp(func(tx *Tx) error {
		obj, err := t.lookupObject(tx)
		if err != nil {
			return err
		}
		rref, err = obj.GetRootRef()
		return err
	})
	return rref, err
}

// SetRootRef changes the root reference of the object.
func (t *EngineTxObjectState) SetRootRef(nref *bucket.ObjectRef) error {
	if t.t.GetReadOnly() {
		return tx.ErrNotWrite
	}

	return t.t.performOp(func(tx *Tx) error {
		obj, err := t.lookupObject(tx)
		if err != nil {
			return err
		}
		return obj.SetRootRef(nref)
	})
}

// ApplyOperation applies an object-specific operation.
// Returns any errors processing the operation.
func (t *EngineTxObjectState) ApplyOperation(op world.ObjectOp) error {
	if t.t.GetReadOnly() {
		return tx.ErrNotWrite
	}

	return t.t.performOp(func(tx *Tx) error {
		obj, err := t.lookupObject(tx)
		if err != nil {
			return err
		}
		return obj.ApplyOperation(op)
	})
}

// lookupObject returns the object or ErrObjectNotFound
func (t *EngineTxObjectState) lookupObject(tx *Tx) (world.ObjectState, error) {
	obj, found, err := tx.GetObject(t.key)
	if err != nil {
		return nil, err
	}
	// note: to create a EngineTxObjectState, we previously checked
	// if the object key exists. it must have been deleted since.
	if !found {
		return nil, world.ErrObjectNotFound
	}
	return obj, nil
}

// _ is a type assertion
var _ world.ObjectState = (*EngineTxObjectState)(nil)
