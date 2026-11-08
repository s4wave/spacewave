package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
)

// EngineTxObjectState is an ObjectState handle attached to a EngineTx.
type EngineTxObjectState struct {
	// t is the engine transaction the handle belongs to
	t *EngineTx
	// key is the object key
	key string
	// obj is the object handle cached by a write transaction, or nil
	obj world.ObjectState
}

// newEngineTxObjectState constructs a new EngineTx ObjectState object.
func newEngineTxObjectState(t *EngineTx, key string, obj world.ObjectState) *EngineTxObjectState {
	return &EngineTxObjectState{t: t, key: key, obj: obj}
}

// GetKey returns the key this state object is for.
func (t *EngineTxObjectState) GetKey() string {
	return t.key
}

// GetRootRef returns the root reference of the object.
func (t *EngineTxObjectState) GetRootRef(ctx context.Context) (ref *bucket.ObjectRef, rev uint64, err error) {
	err = t.withObject(ctx, func(obj world.ObjectState) error {
		ref, rev, err = obj.GetRootRef(ctx)
		return err
	})
	return ref, rev, err
}

// GetModSeqno returns the World seqno after the last commit that changed the
// object.
func (t *EngineTxObjectState) GetModSeqno(ctx context.Context) (seqno uint64, err error) {
	err = t.withObject(ctx, func(obj world.ObjectState) error {
		seqno, err = obj.GetModSeqno(ctx)
		return err
	})
	return seqno, err
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
// If the ref is empty, will default to the object RootRef.
// If the ref Bucket ID is empty, uses the same bucket + volume as the world.
// The lookup cursor will be released after cb returns.
func (t *EngineTxObjectState) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	if ref.GetEmpty() {
		var err error
		ref, _, err = t.GetRootRef(ctx)
		if err != nil {
			return err
		}
	}
	return t.t.AccessWorldState(ctx, ref, cb)
}

// SetRootRef changes the root reference of the object.
func (t *EngineTxObjectState) SetRootRef(ctx context.Context, nref *bucket.ObjectRef) (rev uint64, err error) {
	if t.t.GetReadOnly() {
		return 0, tx.ErrNotWrite
	}
	err = t.withObject(ctx, func(obj world.ObjectState) error {
		rev, err = obj.SetRootRef(ctx, nref)
		return err
	})
	return rev, err
}

// ApplyObjectOp applies a batch operation at the object level.
// The handling of the operation is operation-type specific.
// Returns the revision following the operation execution.
// If nil is returned for the error, implies success.
func (t *EngineTxObjectState) ApplyObjectOp(
	ctx context.Context,
	op world.Operation,
	opSender peer.ID,
) (rev uint64, sysErr bool, err error) {
	if t.t.GetReadOnly() {
		return 0, false, tx.ErrNotWrite
	}
	err = t.withObject(ctx, func(obj world.ObjectState) error {
		rev, sysErr, err = obj.ApplyObjectOp(ctx, op, opSender)
		return err
	})
	return rev, sysErr, err
}

// IncrementRev increments the revision of the object.
// Returns the new latest revision.
func (t *EngineTxObjectState) IncrementRev(ctx context.Context) (rev uint64, err error) {
	if t.t.GetReadOnly() {
		return 0, tx.ErrNotWrite
	}
	err = t.withObject(ctx, func(obj world.ObjectState) error {
		rev, err = obj.IncrementRev(ctx)
		return err
	})
	return rev, err
}

// WaitRev waits until the object rev is >= the specified.
// Returns ErrObjectNotFound if the object is deleted.
// If ignoreNotFound is set, waits for the object to exist.
// Immutable read transactions cannot advance. Live watches use EngineWorldState.
func (t *EngineTxObjectState) WaitRev(
	ctx context.Context,
	rev uint64,
	ignoreNotFound bool,
) (uint64, error) {
	// A writer can advance within its pending transaction. Count its changes
	// before reading the revision.
	writeTx := t.t.writeTx
	var changes uint64
	if writeTx != nil {
		changes = writeTx.state.getChanges()
	}

	for {
		_, currRev, err := t.GetRootRef(ctx)
		if err != nil {
			return 0, err
		}
		if currRev >= rev {
			return currRev, nil
		}

		// A snapshot can only satisfy revisions it already contains.
		if writeTx == nil {
			<-ctx.Done()
			return 0, ctx.Err()
		}
		changes, err = writeTx.state.waitChanges(ctx, changes)
		if err != nil {
			return 0, err
		}
	}
}

// withObject runs cb on the object through the transaction's performOp. A
// write transaction lends its cached handle, loading and caching it on first
// use; a read snapshot loads a handle for each attempt and releases it after
// cb.
func (t *EngineTxObjectState) withObject(ctx context.Context, cb func(world.ObjectState) error) error {
	return t.t.performOp(ctx, func(tx *Tx) error {
		// Reuse the cached object while the write transaction holds it.
		if t.obj != nil && t.t.writeTx != nil {
			return cb(t.obj)
		}

		// Load the object and cache it on a write transaction.
		obj, found, err := tx.GetObject(ctx, t.key)
		if err != nil {
			world.ReleaseObjectState(obj)
			return err
		}

		// note: to create a EngineTxObjectState, we previously checked
		// if the object key exists. it must have been deleted since.
		if !found {
			return world.ErrObjectNotFound
		}
		if t.t.writeTx != nil {
			t.obj = obj
		} else {
			defer world.ReleaseObjectState(obj)
		}
		return cb(obj)
	})
}

// Release releases the cached write handle owned by this wrapper.
func (t *EngineTxObjectState) Release() {
	world.ReleaseObjectState(t.obj)
}

// _ is a type assertion
var _ world.ObjectState = (*EngineTxObjectState)(nil)
