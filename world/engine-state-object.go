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
func (e *engineWorldStateObject) GetRootRef() (*bucket.ObjectRef, error) {
	var outRef *bucket.ObjectRef
	err := e.e.performOp(false, func(tx Tx) error {
		obj, err := MustGetObject(tx, e.key)
		if err != nil {
			return err
		}
		outRef, err = obj.GetRootRef()
		return err
	})
	return outRef, err
}

// SetRootRef changes the root reference of the object.
func (e *engineWorldStateObject) SetRootRef(nref *bucket.ObjectRef) error {
	return e.e.performOp(true, func(tx Tx) error {
		obj, err := MustGetObject(tx, e.key)
		if err != nil {
			return err
		}
		return obj.SetRootRef(nref)
	})
}

// ApplyOperation applies an object-specific operation.
// Returns any errors processing the operation.
func (e *engineWorldStateObject) ApplyOperation(op ObjectOp) error {
	return e.e.performOp(true, func(tx Tx) error {
		obj, err := MustGetObject(tx, e.key)
		if err != nil {
			return err
		}
		return obj.ApplyOperation(op)
	})
}

// _ is a type assertion
var (
	_ ObjectState = ((*engineWorldStateObject)(nil))
)
