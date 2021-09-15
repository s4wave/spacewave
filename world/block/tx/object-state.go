package world_block_tx

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/tx"
	"github.com/aperturerobotics/hydra/world"
)

// ObjectState is an Object attached to a WorldState.
type ObjectState struct {
	// w is the WorldState
	w *WorldState
	// key is the object key
	key string
	// o is the underlying object
	o world.ObjectState
}

// NewObjectState returns a new Object wrapped with a tx.
func NewObjectState(w *WorldState, key string, o world.ObjectState) *ObjectState {
	return &ObjectState{w: w, key: key, o: o}
}

// GetKey returns the key this state object is for.
func (t *ObjectState) GetKey() string {
	return t.key
}

// GetRootRef returns the root reference of the object.
func (t *ObjectState) GetRootRef() (*bucket.ObjectRef, uint64, error) {
	t.w.mtx.Lock()
	defer t.w.mtx.Unlock()

	if t.w.discarded {
		return nil, 0, tx.ErrDiscarded
	}

	return t.o.GetRootRef()
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
// If the ref is empty, will default to the object RootRef.
// If the ref Bucket ID is empty, uses the same bucket + volume as the world.
// The lookup cursor will be released after cb returns.
func (t *ObjectState) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	return t.o.AccessWorldState(ctx, ref, cb)
}

// SetRootRef changes the root reference of the object.
func (t *ObjectState) SetRootRef(nref *bucket.ObjectRef) (uint64, error) {
	if !t.w.write {
		return 0, tx.ErrNotWrite
	}

	tt, err := NewTxObjectSet(t.key, nref)
	if err != nil {
		return 0, err
	}

	t.w.mtx.Lock()
	defer t.w.mtx.Unlock()

	if t.w.discarded {
		return 0, tx.ErrDiscarded
	}

	seqno, err := t.o.SetRootRef(nref)
	if err != nil {
		return 0, err
	}

	t.w.txBatch.Txs = append(t.w.txBatch.Txs, tt)
	return seqno, nil
}

// ApplyObjectOp applies a batch operation at the object level.
// The handling of the operation is operation-type specific.
// Returns the revision following the operation execution.
// If nil is returned for the error, implies success.
func (t *ObjectState) ApplyObjectOp(op world.Operation, opSender peer.ID) (uint64, bool, error) {
	if !t.w.write {
		return 0, false, tx.ErrNotWrite
	}
	if op == nil {
		return 0, false, world.ErrEmptyOp
	}

	operationTypeID := op.GetOperationTypeId()
	tt, err := NewTxApplyObjectOp(operationTypeID, op, t.key)
	if err != nil {
		return 0, false, err
	}

	t.w.mtx.Lock()
	defer t.w.mtx.Unlock()

	if t.w.discarded {
		return 0, false, tx.ErrDiscarded
	}

	seqno, sysErr, err := t.o.ApplyObjectOp(op, opSender)
	if err == nil {
		t.w.txBatch.Txs = append(t.w.txBatch.Txs, tt)
		if seqno > t.w.seqno {
			t.w.seqno = seqno
		} else {
			t.w.seqno++
			seqno = t.w.seqno
		}
	}
	return seqno, sysErr, err
}

// IncrementRev increments the revision of the object.
// Returns the new latest revision.
func (t *ObjectState) IncrementRev() (uint64, error) {
	if !t.w.write {
		return 0, tx.ErrNotWrite
	}

	tt, err := NewTxObjectIncRev(t.key)
	if err != nil {
		return 0, err
	}

	t.w.mtx.Lock()
	defer t.w.mtx.Unlock()

	if t.w.discarded {
		return 0, tx.ErrDiscarded
	}

	orev, err := t.o.IncrementRev()
	if err != nil {
		return 0, err
	}

	t.w.txBatch.Txs = append(t.w.txBatch.Txs, tt)
	return orev, nil
}

// WaitRev waits until the object rev is >= the specified.
// Returns ErrObjectNotFound if the object is deleted.
// If ignoreNotFound is set, waits for the object to exist.
// Returns the new rev.
func (t *ObjectState) WaitRev(
	ctx context.Context,
	rev uint64,
	ignoreNotFound bool,
) (uint64, error) {
	return t.o.WaitRev(ctx, rev, ignoreNotFound)
}

// _ is a type assertion
var _ world.ObjectState = ((*ObjectState)(nil))
