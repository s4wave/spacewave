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
func (t *TxObjectState) GetRootRef() (*bucket.ObjectRef, uint64, error) {
	t.tx.rmtx.Lock()
	defer t.tx.rmtx.Unlock()

	if t.tx.discarded {
		return nil, 0, tx.ErrDiscarded
	}

	return t.o.GetRootRef()
}

// SetRootRef changes the root reference of the object.
func (t *TxObjectState) SetRootRef(nref *bucket.ObjectRef) (uint64, error) {
	t.tx.rmtx.Lock()
	defer t.tx.rmtx.Unlock()

	if t.tx.discarded {
		return 0, tx.ErrDiscarded
	}

	return t.o.SetRootRef(nref)
}

// ApplyOperation applies an object-specific operation.
// Returns any errors processing the operation.
func (t *TxObjectState) ApplyOperation(op world.ObjectOp) (uint64, error) {
	t.tx.rmtx.Lock()
	defer t.tx.rmtx.Unlock()

	if t.tx.discarded {
		return 0, tx.ErrDiscarded
	}

	return t.o.ApplyOperation(op)
}

// IncrementRev increments the revision of the object.
// Returns the new latest revision.
func (t *TxObjectState) IncrementRev() (uint64, error) {
	t.tx.rmtx.Lock()
	defer t.tx.rmtx.Unlock()

	if t.tx.discarded {
		return 0, tx.ErrDiscarded
	}

	return t.o.IncrementRev()
}

// _ is a type assertion
var _ world.ObjectState = ((*TxObjectState)(nil))
