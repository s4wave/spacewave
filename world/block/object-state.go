package world_block

import (
	"context"
	"errors"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
)

// ObjectState implements the ObjectState interface attached to block cursor.
type ObjectState struct {
	w   *WorldState
	bcs *block.Cursor
}

// NewObjectState constructs a new ObjectState from a block cursor and world state.
func NewObjectState(w *WorldState, bcs *block.Cursor) (*ObjectState, error) {
	s := &ObjectState{w: w, bcs: bcs}
	return s, nil
}

// GetRootRef returns the root reference of the object.
func (o *ObjectState) GetRootRef() (*bucket.ObjectRef, uint64, error) {
	root, err := o.getRoot()
	if err != nil {
		return nil, 0, err
	}
	return root.GetRootRef(), root.GetRev(), nil
}

// SetRootRef changes the root reference of the object.
func (o *ObjectState) SetRootRef(nref *bucket.ObjectRef) (uint64, error) {
	if err := nref.Validate(); err != nil {
		return 0, err
	}
	root, err := o.getRoot()
	if err != nil {
		return 0, err
	}
	if root.GetRootRef().EqualsRef(nref) {
		// no-op
		return root.GetRev(), nil
	}
	root.RootRef = nref
	root.Rev++
	r := root.Rev
	o.bcs.SetBlock(root, true)
	return r, nil
}

// ApplyOperation applies an object-specific operation.
// Returns any errors processing the operation.
func (o *ObjectState) ApplyOperation(op world.ObjectOp) (uint64, error) {
	// TODO
	return 0, errors.New("TODO world/block object-state apply operation")
}

// IncrementRev increments the revision of the object.
// Returns the new latest revision.
func (o *ObjectState) IncrementRev() (uint64, error) {
	root, err := o.getRoot()
	if err != nil {
		return 0, err
	}
	root.Rev++
	nrev := root.Rev
	o.bcs.SetBlock(root, true)
	return nrev, nil
}

// WaitRev waits until the object rev is >= the specified.
// Returns ErrObjectNotFound if the object is deleted.
// If ignoreNotFound is set, waits for the object to exist.
// Returns the new rev.
func (o *ObjectState) WaitRev(
	ctx context.Context,
	rev uint64,
	ignoreNotFound bool,
) (uint64, error) {
	// TODO this will likely be: wait for a local writer to increment rev
	// i.e. it will wait for someone else to change the block graph
	return 0, errors.New("TODO world/block object-state wait rev")
}

// getRoot unmarshals root from the block cursor
func (o *ObjectState) getRoot() (*Object, error) {
	obji, err := o.bcs.Unmarshal(NewObjectBlock)
	if err != nil {
		return nil, err
	}
	if obji == nil {
		return nil, world.ErrObjectNotFound
	}
	v, ok := obji.(*Object)
	if !ok {
		return nil, block.ErrUnexpectedType
	}
	return v, nil
}

// _ is a type assertion
var _ world.ObjectState = ((*ObjectState)(nil))
