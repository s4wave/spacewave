package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
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

// Release releases the object handle owned by this transaction wrapper.
func (t *TxObjectState) Release() {
	world.ReleaseObjectState(t.o)
}

// GetKey returns the key this state object is for.
func (t *TxObjectState) GetKey() string {
	return t.key
}

// GetRootRef returns the root reference of the object.
func (t *TxObjectState) GetRootRef(ctx context.Context) (ref *bucket.ObjectRef, rev uint64, err error) {
	err = t.tx.lockedLive(ctx, func() error {
		ref, rev, err = t.o.GetRootRef(ctx)
		return err
	})
	return ref, rev, err
}

// GetModSeqno returns the World seqno after the last commit that changed the
// object.
func (t *TxObjectState) GetModSeqno(ctx context.Context) (seqno uint64, err error) {
	err = t.tx.lockedLive(ctx, func() error {
		seqno, err = t.o.GetModSeqno(ctx)
		return err
	})
	return seqno, err
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
	return lockedValue(ctx, t.tx, true, func() (uint64, error) {
		return t.o.SetRootRef(ctx, nref)
	})
}

// ApplyObjectOp applies a batch operation at the object level.
// The handling of the operation is operation-type specific.
// Returns the revision following the operation execution.
// If nil is returned for the error, implies success.
func (t *TxObjectState) ApplyObjectOp(ctx context.Context, op world.Operation, opSender peer.ID) (rev uint64, sysErr bool, err error) {
	err = t.tx.locked(ctx, true, func() error {
		rev, sysErr, err = t.o.ApplyObjectOp(ctx, op, opSender)
		return err
	})
	return rev, sysErr, err
}

// IncrementRev increments the revision of the object.
// Returns the new latest revision.
func (t *TxObjectState) IncrementRev(ctx context.Context) (uint64, error) {
	return lockedValue(ctx, t.tx, true, func() (uint64, error) {
		return t.o.IncrementRev(ctx)
	})
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
	for {
		var changes uint64
		var currObjRev uint64
		err := t.tx.locked(ctx, false, func() error {
			// Count the transaction's changes before reading the revision.
			changes = t.tx.state.getChanges()

			// Load the object and require that it exists.
			obj, err := t.tx.state.mustGetObject(ctx, t.key)
			if err != nil {
				return err
			}

			// Read the object's current revision.
			_, currObjRev, err = obj.GetRootRef(ctx)
			if err != nil {
				return err
			}

			return nil
		})
		if err != nil {
			return 0, err
		}
		if currObjRev >= rev {
			return currObjRev, nil
		}

		_, err = t.tx.state.waitChanges(ctx, changes)
		if err != nil {
			return 0, err
		}
	}
}

// _ is a type assertion
var _ world.ObjectState = (*TxObjectState)(nil)
