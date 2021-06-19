package world_block

import (
	"context"
	"errors"

	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/tx"
	"github.com/aperturerobotics/hydra/world"
)

// TxObjectState is an Object attached to a Tx.
// Concurrent safe guarded by rmtx on the Tx.
type TxObjectState struct {
	// tx is the transaction
	tx *Tx
	// key is the object key
	key string
	// o is the object
	o world.ObjectState
}

// NewTxObjectState returns a new Object wrapped with a tx.
func NewTxObjectState(t *Tx, key string, o world.ObjectState) *TxObjectState {
	return &TxObjectState{tx: t, key: key, o: o}
}

// GetKey returns the key this state object is for.
func (t *TxObjectState) GetKey() string {
	return t.key
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

// AccessWorldState builds a bucket lookup cursor with an optional ref.
// If the ref is empty, will default to the object RootRef.
// If the ref Bucket ID is empty, uses the same bucket + volume as the world.
// The lookup cursor will be released after cb returns.
func (t *TxObjectState) AccessWorldState(
	ctx context.Context,
	write bool,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	return t.o.AccessWorldState(ctx, write, ref, cb)
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

// ApplyObjectOp applies a batch operation at the object level.
// The handling of the operation is operation-type specific.
// Returns the revision following the operation execution.
// If nil is returned for the error, implies success.
func (t *TxObjectState) ApplyObjectOp(operationTypeID string, op world.Operation) (uint64, error) {
	t.tx.rmtx.Lock()
	defer t.tx.rmtx.Unlock()

	if t.tx.discarded {
		return 0, tx.ErrDiscarded
	}

	return t.o.ApplyObjectOp(operationTypeID, op)
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

// WaitRev waits until the object rev is >= the specified.
// Returns ErrObjectNotFound if the object is deleted.
// If ignoreNotFound is set, waits for the object to exist.
// Returns the new rev.
func (t *TxObjectState) WaitRev(
	ctx context.Context,
	rev uint64,
	ignoreNotFound bool,
) (uint64, error) {
	t.tx.rmtx.Lock()
	defer t.tx.rmtx.Unlock()

	// t.tx.state.GetSeqno()
	return 0, errors.New("TODO tx object state waitrev")
}

// _ is a type assertion
var _ world.ObjectState = ((*TxObjectState)(nil))
