package world_block

import (
	"github.com/aperturerobotics/hydra/block/byteslice"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	"github.com/cayleygraph/quad"
)

// CreateObject creates an empty object with a key.
// Returns ErrObjectExists if the object already exists.
func (t *WorldState) CreateObject(key string, rootRef *bucket.ObjectRef) (world.ObjectState, error) {
	ot := t.objTree
	k := t.buildObjectKey(key)
	exists, err := ot.Exists(k)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, world.ErrObjectExists
	}
	obj := NewObject(key, rootRef)
	nbcs := t.bcs.Detach(false)
	nbcs.SetBlock(obj, true)
	_, _, err = t.objTree.SetCursorAsRef(k, nbcs)
	if err != nil {
		return nil, err
	}
	return NewObjectState(t, nbcs)
}

// GetObject looks up an object by key.
// Returns nil, false if not found.
func (t *WorldState) GetObject(key string) (world.ObjectState, bool, error) {
	ot := t.objTree
	k := t.buildObjectKey(key)
	_, vk, err := ot.GetWithCursor(k)
	if err != nil || vk == nil {
		return nil, false, err
	}
	br, err := byteslice.ByteSliceToRef(vk, true)
	if err != nil {
		return nil, true, err
	}
	bcs := vk.FollowRef(1, br)
	ost, err := NewObjectState(t, bcs)
	return ost, true, err
}

// DeleteObject deletes an object and associated graph quads by ID.
// Calls DeleteGraphObject internally.
// Returns false, nil if not found.
func (t *WorldState) DeleteObject(key string) (bool, error) {
	ot := t.objTree
	k := t.buildObjectKey(key)
	exists, err := ot.Exists(k)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, err
	}
	err = ot.Delete(k)
	if err != nil {
		return true, err
	}
	err = t.DeleteGraphObject(quad.IRI(key).String())
	if err != nil {
		return true, err
	}
	// success
	return true, nil
}

// _ is a type assertion
var _ world.WorldStateObject = ((*WorldState)(nil))
