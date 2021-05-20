package world_block

import (
	"errors"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
)

// ObjectState implements the ObjectState interface attached to block cursor.
type ObjectState struct {
	w    *WorldState
	bcs  *block.Cursor
	root *Object
}

// NewObjectState constructs a new ObjectState from a block cursor and world state.
func NewObjectState(w *WorldState, bcs *block.Cursor) (*ObjectState, error) {
	v, err := bcs.Unmarshal(NewObjectBlock)
	if err != nil {
		return nil, err
	}
	ov, ok := v.(*Object)
	if !ok {
		return nil, block.ErrUnexpectedType
	}
	return &ObjectState{w: w, bcs: bcs, root: ov}, nil
}

// GetRootRef returns the root reference of the object.
func (o *ObjectState) GetRootRef() (*bucket.ObjectRef, error) {
	return o.root.GetRootRef(), nil
}

// SetRootRef changes the root reference of the object.
func (o *ObjectState) SetRootRef(nref *bucket.ObjectRef) error {
	if err := nref.Validate(); err != nil {
		return err
	}
	o.root.RootRef = nref
	o.bcs.SetBlock(o.root, true)
	return nil
}

// ApplyOperation applies an object-specific operation.
// Returns any errors processing the operation.
func (o *ObjectState) ApplyOperation(op world.ObjectOp) error {
	// TODO
	return errors.New("TODO world/block object-state apply operation")
}

// _ is a type assertion
var _ world.ObjectState = ((*ObjectState)(nil))
