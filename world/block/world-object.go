package world_block

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	"github.com/cayleygraph/quad"
)

// CreateObject creates a object with a key and initial root ref.
// Returns ErrObjectExists if the object already exists.
// Appends a OBJECT_SET change to the changelog.
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
	nbcs.ClearAllRefs()
	nbcs.SetBlock(obj, true)
	err = t.objTree.SetCursorAtKey(k, nbcs, false)
	if err != nil {
		return nil, err
	}
	objState, err := NewObjectState(t, nbcs)
	if err != nil {
		return nil, err
	}
	changeBcs, err := t.queueWorldChange(&WorldChange{
		Key:        key,
		ChangeType: WorldChangeType_WorldChange_OBJECT_SET,
	})
	if err != nil {
		return nil, err
	}
	changeBcs.SetRef(5, nbcs)
	return objState, nil
}

// GetObject looks up an object by key.
// Returns nil, false if not found.
func (t *WorldState) GetObject(key string) (world.ObjectState, bool, error) {
	ot := t.objTree
	k := t.buildObjectKey(key)
	bcs, err := ot.GetCursorAtKey(k)
	if err != nil || bcs == nil {
		return nil, false, err
	}
	ost, err := NewObjectState(t, bcs)
	return ost, true, err
}

// DeleteObject deletes an object and associated graph quads by ID.
// Calls DeleteGraphObject internally.
// Returns false, nil if not found.
func (t *WorldState) DeleteObject(key string) (bool, error) {
	ot := t.objTree
	k := t.buildObjectKey(key)
	objState, found, err := t.GetObject(key)
	if err != nil {
		if err != world.ErrObjectNotFound {
			return false, err
		}
	}
	if !found {
		return false, nil
	}
	objs, ok := objState.(*ObjectState)
	if !ok {
		return false, block.ErrUnexpectedType
	}
	nbcs := objs.bcs

	err = t.DeleteGraphObject(quad.IRI(key).String())
	if err != nil {
		return true, err
	}

	err = ot.Delete(k)
	if err != nil {
		return true, err
	}
	changeBcs, err := t.queueWorldChange(&WorldChange{
		Key:        key,
		ChangeType: WorldChangeType_WorldChange_OBJECT_DELETE,
	})
	if err != nil {
		return false, err
	}
	changeBcs.SetRef(7, nbcs)
	// success
	return true, nil
}

// _ is a type assertion
var _ world.WorldStateObject = ((*WorldState)(nil))
