package world

import "github.com/aperturerobotics/hydra/bucket"

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

// _ is a type assertion
var _ ObjectState = ((*engineWorldStateObject)(nil))
