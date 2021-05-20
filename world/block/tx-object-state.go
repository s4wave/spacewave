package world_block

import (
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/tx"
	"github.com/aperturerobotics/hydra/world"
)

// TxObjectState is an Object attached to a Tx.
// Concurrent safe guarded by rmtx on the Tx.
type TxObjectState struct {
	// tx is the transaction
	tx *Tx
	// o is the object
	o world.ObjectState
}

// NewTxObjectState returns a new Object wrapped with a tx.
func NewTxObjectState(t *Tx, o world.ObjectState) *TxObjectState {
	return &TxObjectState{tx: t, o: o}
}

// GetRootRef returns the root reference of the object.
func (t *TxObjectState) GetRootRef() (*bucket.ObjectRef, error) {
	t.tx.rmtx.Lock()
	defer t.tx.rmtx.Unlock()

	if t.tx.discarded {
		return nil, tx.ErrDiscarded
	}

	return t.o.GetRootRef()
}

// SetRootRef changes the root reference of the object.
func (t *TxObjectState) SetRootRef(nref *bucket.ObjectRef) error {
	t.tx.rmtx.Lock()
	defer t.tx.rmtx.Unlock()

	if t.tx.discarded {
		return tx.ErrDiscarded
	}

	return t.o.SetRootRef(nref)
}

// ApplyOperation applies an object-specific operation.
// Returns any errors processing the operation.
func (t *TxObjectState) ApplyOperation(op world.ObjectOp) error {
	t.tx.rmtx.Lock()
	defer t.tx.rmtx.Unlock()

	if t.tx.discarded {
		return tx.ErrDiscarded
	}

	return t.o.ApplyOperation(op)
}

// _ is a type assertion
var _ world.ObjectState = ((*TxObjectState)(nil))
