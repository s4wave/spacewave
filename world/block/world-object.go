package world_block

import (
	"context"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	"github.com/cayleygraph/quad"
)

// CreateObject creates a object with a key and initial root ref.
// Returns ErrObjectExists if the object already exists.
// Appends a OBJECT_SET change to the changelog.
func (t *WorldState) CreateObject(ctx context.Context, key string, rootRef *bucket.ObjectRef) (world.ObjectState, error) {
	ot := t.objTree
	k := t.buildObjectKey(key)
	exists, err := ot.Exists(ctx, k)
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
	err = t.objTree.SetCursorAtKey(ctx, k, nbcs, false)
	if err != nil {
		return nil, err
	}
	objState, err := NewObjectState(ctx, t, nbcs)
	if err != nil {
		return nil, err
	}
	changeBcs, err := t.queueWorldChange(ctx, &WorldChange{
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
func (t *WorldState) GetObject(ctx context.Context, key string) (world.ObjectState, bool, error) {
	ot := t.objTree
	k := t.buildObjectKey(key)
	bcs, err := ot.GetCursorAtKey(ctx, k)
	if err != nil || bcs == nil {
		return nil, false, err
	}
	ost, err := NewObjectState(ctx, t, bcs)
	if err != nil {
		return nil, false, err
	}
	return ost, true, nil
}

// DeleteObject deletes an object and associated graph quads by ID.
// Calls DeleteGraphObject internally.
// Returns false, nil if not found.
func (t *WorldState) DeleteObject(ctx context.Context, key string) (bool, error) {
	ot := t.objTree
	k := t.buildObjectKey(key)
	objState, found, err := t.GetObject(ctx, key)
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

	err = t.DeleteGraphObject(ctx, quad.IRI(key).String())
	if err != nil {
		return true, err
	}

	err = ot.Delete(ctx, k)
	if err != nil {
		return true, err
	}
	changeBcs, err := t.queueWorldChange(ctx, &WorldChange{
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
