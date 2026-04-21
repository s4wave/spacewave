package world

import (
	"context"

	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
)

// engineWorldStateObject is a ObjectState attached to an EngineWorldState.
type engineWorldStateObject struct {
	e   *engineWorldState
	key string
}

// newEngineWorldStateObject creates an object attached to an EngineWorldState.
func newEngineWorldStateObject(e *engineWorldState, key string) *engineWorldStateObject {
	return &engineWorldStateObject{e: e, key: key}
}

// GetKey returns the key this state object is for.
func (e *engineWorldStateObject) GetKey() string {
	return e.key
}

// GetRootRef returns the root reference.
func (e *engineWorldStateObject) GetRootRef(ctx context.Context) (*bucket.ObjectRef, uint64, error) {
	var outRef *bucket.ObjectRef
	var outRev uint64
	err := e.e.performOp(ctx, false, func(tx Tx) error {
		obj, err := MustGetObject(ctx, tx, e.key)
		if err != nil {
			return err
		}
		outRef, outRev, err = obj.GetRootRef(ctx)
		return err
	})
	return outRef, outRev, err
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
// If the ref is empty, will default to the object RootRef.
// If the ref Bucket ID is empty, uses the same bucket + volume as the world.
// The lookup cursor will be released after cb returns.
func (e *engineWorldStateObject) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	return e.e.performOp(ctx, false, func(tx Tx) error {
		if ref.GetEmpty() {
			obj, err := MustGetObject(ctx, tx, e.key)
			if err != nil {
				return err
			}
			rootRef, _, err := obj.GetRootRef(ctx)
			if err != nil {
				return err
			}
			ref = rootRef
		}
		return tx.AccessWorldState(ctx, ref, cb)
	})
}

// SetRootRef changes the root reference of the object.
func (e *engineWorldStateObject) SetRootRef(ctx context.Context, nref *bucket.ObjectRef) (uint64, error) {
	var outRev uint64
	err := e.e.performOp(ctx, true, func(tx Tx) error {
		obj, berr := MustGetObject(ctx, tx, e.key)
		if berr == nil {
			outRev, berr = obj.SetRootRef(ctx, nref)
		}
		return berr
	})
	return outRev, err
}

// ApplyObjectOp applies a batch operation at the object level.
// The handling of the operation is operation-type specific.
// Returns the revision following the operation execution.
// If nil is returned for the error, implies success.
func (e *engineWorldStateObject) ApplyObjectOp(
	ctx context.Context,
	op Operation,
	opSender peer.ID,
) (uint64, bool, error) {
	var outRev uint64
	var outSysErr bool
	err := e.e.performOp(ctx, true, func(tx Tx) error {
		obj, berr := MustGetObject(ctx, tx, e.key)
		if berr == nil {
			outRev, outSysErr, berr = obj.ApplyObjectOp(ctx, op, opSender)
		}
		return berr
	})
	return outRev, outSysErr, err
}

// IncrementRev increments the revision of the object.
// Returns the new latest revision.
func (e *engineWorldStateObject) IncrementRev(ctx context.Context) (uint64, error) {
	var val uint64
	err := e.e.performOp(ctx, true, func(tx Tx) error {
		obj, berr := MustGetObject(ctx, tx, e.key)
		if berr == nil {
			val, berr = obj.IncrementRev(ctx)
		}
		return berr
	})
	return val, err
}

// WaitRev waits until the object rev is >= the specified.
// Returns ErrObjectNotFound if the object is deleted.
// If ignoreNotFound is set, waits for the object to exist.
// Returns the new rev.
func (e *engineWorldStateObject) WaitRev(
	ctx context.Context,
	rev uint64,
	ignoreNotFound bool,
) (uint64, error) {
	for {
		if err := ctx.Err(); err != nil {
			return 0, ctx.Err()
		}
		var found bool
		var nSeqno uint64
		var currRev uint64
		err := e.e.performOp(ctx, false, func(tx Tx) error {
			seqno, err := tx.GetSeqno(ctx)
			if err != nil {
				return err
			}
			nSeqno = seqno + 1
			objState, objFound, err := tx.GetObject(ctx, e.key)
			if err != nil {
				return err
			}
			found = objFound
			if !objFound {
				currRev = 0
			} else {
				_, currRev, err = objState.GetRootRef(ctx)
				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return 0, err
		}
		if found {
			if currRev >= rev {
				return currRev, nil
			}
		} else if !ignoreNotFound {
			return 0, ErrObjectNotFound
		}

		// currRev < rev: wait for currRev >= rev
		// ignoreNotFound: wait for object to exist
		_, err = e.e.e.WaitSeqno(ctx, nSeqno)
		if err != nil {
			return 0, err
		}
	}
}

// _ is a type assertion
var _ ObjectState = ((*engineWorldStateObject)(nil))
