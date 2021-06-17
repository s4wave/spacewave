package world

import (
	"context"

	"github.com/aperturerobotics/hydra/bucket"
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

// GetRootRef returns the root reference.
func (e *engineWorldStateObject) GetRootRef() (*bucket.ObjectRef, uint64, error) {
	var outRef *bucket.ObjectRef
	var outRev uint64
	err := e.e.performOp(false, func(tx Tx) error {
		obj, err := MustGetObject(tx, e.key)
		if err != nil {
			return err
		}
		outRef, outRev, err = obj.GetRootRef()
		return err
	})
	return outRef, outRev, err
}

// SetRootRef changes the root reference of the object.
func (e *engineWorldStateObject) SetRootRef(nref *bucket.ObjectRef) (uint64, error) {
	var outRev uint64
	err := e.e.performOp(true, func(tx Tx) error {
		obj, berr := MustGetObject(tx, e.key)
		if berr == nil {
			outRev, berr = obj.SetRootRef(nref)
		}
		return berr
	})
	return outRev, err
}

// ApplyOperation applies an object-specific operation.
// Returns any errors processing the operation.
func (e *engineWorldStateObject) ApplyOperation(op ObjectOp) (uint64, error) {
	var outRev uint64
	err := e.e.performOp(true, func(tx Tx) error {
		obj, berr := MustGetObject(tx, e.key)
		if berr == nil {
			outRev, berr = obj.ApplyOperation(op)
		}
		return berr
	})
	return outRev, err
}

// IncrementRev increments the revision of the object.
// Returns the new latest revision.
func (e *engineWorldStateObject) IncrementRev() (uint64, error) {
	var val uint64
	err := e.e.performOp(true, func(tx Tx) error {
		obj, berr := MustGetObject(tx, e.key)
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
func (e *engineWorldStateObject) WaitRev(
	ctx context.Context,
	rev uint64,
	ignoreNotFound bool,
) (uint64, error) {
	for {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}
		var found bool
		var nSeqno uint64 // TODO
		var currRev uint64
		err := e.e.performOp(false, func(tx Tx) error {
			seqno, err := tx.GetSeqno()
			if err != nil {
				return err
			}
			nSeqno = seqno + 1
			objState, objFound, err := tx.GetObject(e.key)
			if err != nil {
				return err
			}
			found = objFound
			if !objFound {
				currRev = 0
			} else {
				_, currRev, err = objState.GetRootRef()
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
