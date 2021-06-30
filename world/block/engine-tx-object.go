package world_block

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
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

// GetKey returns the key this state object is for.
func (t *EngineTxObjectState) GetKey() string {
	return t.key
}

// GetRootRef returns the root reference of the object.
func (t *EngineTxObjectState) GetRootRef() (*bucket.ObjectRef, uint64, error) {
	var rref *bucket.ObjectRef
	var outRev uint64
	err := t.t.performOp(func(tx *Tx) error {
		obj, err := t.lookupObject(tx)
		if err != nil {
			return err
		}
		rref, outRev, err = obj.GetRootRef()
		return err
	})
	return rref, outRev, err
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
	return t.t.AccessWorldState(ctx, ref, cb)
}

// SetRootRef changes the root reference of the object.
func (t *EngineTxObjectState) SetRootRef(nref *bucket.ObjectRef) (uint64, error) {
	if t.t.GetReadOnly() {
		return 0, tx.ErrNotWrite
	}

	var outRev uint64
	err := t.t.performOp(func(tx *Tx) error {
		obj, berr := t.lookupObject(tx)
		if berr == nil {
			outRev, berr = obj.SetRootRef(nref)
		}
		return berr
	})
	return outRev, err
}

// ApplyObjectOp applies a batch operation at the object level.
// The handling of the operation is operation-type specific.
// Returns the revision following the operation execution.
// If nil is returned for the error, implies success.
func (t *EngineTxObjectState) ApplyObjectOp(
	operationTypeID string,
	op world.Operation,
	opSender peer.ID,
) (uint64, error) {
	if t.t.GetReadOnly() {
		return 0, tx.ErrNotWrite
	}

	var outRev uint64
	err := t.t.performOp(func(tx *Tx) error {
		obj, berr := t.lookupObject(tx)
		if berr == nil {
			outRev, berr = obj.ApplyObjectOp(operationTypeID, op, opSender)
		}
		return berr
	})
	return outRev, err
}

// IncrementRev increments the revision of the object.
// Returns the new latest revision.
func (t *EngineTxObjectState) IncrementRev() (uint64, error) {
	if t.t.GetReadOnly() {
		return 0, tx.ErrNotWrite
	}

	var val uint64
	err := t.t.performOp(func(tx *Tx) error {
		obj, berr := t.lookupObject(tx)
		if berr == nil {
			val, berr = obj.IncrementRev()
		}
		return berr
	})
	return val, err
}

// WaitRev waits until the object rev is >= the specified.
// Returns ErrObjectNotFound if the object is deleted.
// If ignoreNotFound is set, waits for the object to exist.
// Returns the new rev.
func (t *EngineTxObjectState) WaitRev(
	ctx context.Context,
	rev uint64,
	ignoreNotFound bool,
) (uint64, error) {
	seqno, err := t.t.GetSeqno()
	if err != nil {
		return 0, err
	}
	for {
		_, currRev, err := t.GetRootRef()
		if err != nil {
			return 0, err
		}

		if currRev >= rev {
			return currRev, nil
		}

		seqno, err = t.t.engine.WaitSeqno(ctx, seqno+1)
		if err != nil {
			return 0, err
		}
	}
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
