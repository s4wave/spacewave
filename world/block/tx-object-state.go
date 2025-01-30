package world_block

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
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
func (t *TxObjectState) GetRootRef(ctx context.Context) (*bucket.ObjectRef, uint64, error) {
	unlock, err := t.tx.rmtx.Lock(ctx, false)
	if err != nil {
		return nil, 0, err
	}
	defer unlock()

	if t.tx.state.discarded.Load() {
		return nil, 0, tx.ErrDiscarded
	}

	return t.o.GetRootRef(ctx)
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
// If the ref is empty, will default to the object RootRef.
// If the ref Bucket ID is empty, uses the same bucket + volume as the world.
// The lookup cursor will be released after cb returns.
func (t *TxObjectState) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	return t.o.AccessWorldState(ctx, ref, cb)
}

// SetRootRef changes the root reference of the object.
func (t *TxObjectState) SetRootRef(ctx context.Context, nref *bucket.ObjectRef) (uint64, error) {
	unlock, err := t.tx.rmtx.Lock(ctx, true)
	if err != nil {
		return 0, err
	}
	defer unlock()

	return t.o.SetRootRef(ctx, nref)
}

// ApplyObjectOp applies a batch operation at the object level.
// The handling of the operation is operation-type specific.
// Returns the revision following the operation execution.
// If nil is returned for the error, implies success.
func (t *TxObjectState) ApplyObjectOp(ctx context.Context, op world.Operation, opSender peer.ID) (uint64, bool, error) {
	unlock, err := t.tx.rmtx.Lock(ctx, true)
	if err != nil {
		return 0, false, err
	}
	defer unlock()

	return t.o.ApplyObjectOp(ctx, op, opSender)
}

// IncrementRev increments the revision of the object.
// Returns the new latest revision.
func (t *TxObjectState) IncrementRev(ctx context.Context) (uint64, error) {
	unlock, err := t.tx.rmtx.Lock(ctx, true)
	if err != nil {
		return 0, err
	}
	defer unlock()

	return t.o.IncrementRev(ctx)
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
	// t.tx.state.WaitSeqno(ctx context.Context, value uint64)
	return t.o.WaitRev(ctx, rev, ignoreNotFound)
}

// _ is a type assertion
var _ world.ObjectState = ((*TxObjectState)(nil))
