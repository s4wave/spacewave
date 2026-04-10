package world_block

import (
	"context"
	"runtime/trace"

	"github.com/aperturerobotics/hydra/block"
	block_gc "github.com/aperturerobotics/hydra/block/gc"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/tx"
	"github.com/aperturerobotics/hydra/world"
)

// CreateObject creates a object with a key and initial root ref.
// Returns ErrObjectExists if the object already exists.
// Appends a OBJECT_SET change to the changelog.
func (t *WorldState) CreateObject(ctx context.Context, key string, rootRef *bucket.ObjectRef) (world.ObjectState, error) {
	if !t.write {
		return nil, tx.ErrNotWrite
	}
	if t.discarded.Load() {
		return nil, tx.ErrDiscarded
	}

	ot := t.objTree
	k := []byte(objectKeyPrefix + key)
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

	// GC: world -> object, object -> root block.
	if rg := t.refGraph; rg != nil {
		objIRI := block_gc.ObjectIRI(key)
		if err := rg.AddRef(ctx, "world", objIRI); err != nil {
			return nil, err
		}
		rootBlockRef := rootRef.GetRootRef()
		if err := rg.AddObjectRoot(ctx, key, rootBlockRef); err != nil {
			return nil, err
		}
		if rootBlockRef != nil && !rootBlockRef.GetEmpty() {
			_ = rg.RemoveRef(ctx, block_gc.NodeUnreferenced, block_gc.BlockIRI(rootBlockRef))
		}
	}

	return objState, nil
}

// GetObject looks up an object by key.
// Returns nil, false if not found.
func (t *WorldState) GetObject(ctx context.Context, key string) (world.ObjectState, bool, error) {
	val, ok, err := t.getObject(ctx, key)
	if val == nil {
		return nil, ok, err
	}
	return val, ok, err
}

// getObject looks up an object by key.
// Returns nil, false if not found.
func (t *WorldState) getObject(ctx context.Context, key string) (*ObjectState, bool, error) {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/world-state/get-object")
	defer task.End()

	if t.discarded.Load() {
		return nil, false, tx.ErrDiscarded
	}
	ot := t.objTree
	k := []byte(objectKeyPrefix + key)
	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/world-state/get-object/get-cursor-at-key")
	bcs, err := ot.GetCursorAtKey(taskCtx, k)
	subtask.End()
	if err != nil || bcs == nil {
		return nil, false, err
	}
	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/get-object/new-object-state")
	ost, err := NewObjectState(taskCtx, t, bcs)
	subtask.End()
	if err != nil {
		return nil, false, err
	}
	return ost, true, nil
}

// mustGetObject returns an error if not found.
func (t *WorldState) mustGetObject(ctx context.Context, key string) (*ObjectState, error) {
	obj, found, err := t.getObject(ctx, key)
	if err == nil && !found {
		err = world.ErrObjectNotFound
	}
	if err != nil {
		return nil, err
	}
	return obj, nil
}

// IterateObjects returns an iterator with the given object key prefix.
// The prefix is NOT clipped from the output keys.
// Keys are returned in sorted order.
// Must call Next() or Seek() before valid.
// Call Close when done with the iterator.
// Any init errors will be available via the iterator's Err() method.
func (t *WorldState) IterateObjects(ctx context.Context, prefix string, reversed bool) world.ObjectIterator {
	return NewObjectIterator(t, ctx, prefix, reversed)
}

// DeleteObject deletes an object and associated graph quads by ID.
// Calls DeleteGraphObject internally.
// Returns false, nil if not found.
func (t *WorldState) DeleteObject(ctx context.Context, key string) (bool, error) {
	if !t.write {
		return false, tx.ErrNotWrite
	}
	if t.discarded.Load() {
		return false, tx.ErrDiscarded
	}

	ot := t.objTree
	k := []byte(objectKeyPrefix + key)

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

	// GC: remove world -> object edge, mark object unreferenced.
	if rg := t.refGraph; rg != nil {
		objIRI := block_gc.ObjectIRI(key)
		if err := rg.RemoveRef(ctx, "world", objIRI); err != nil {
			return false, err
		}
		if err := rg.AddRef(ctx, block_gc.NodeUnreferenced, objIRI); err != nil {
			return false, err
		}
	}

	// delete any graph links with the object as subject or object
	err = t.DeleteGraphObject(ctx, key)
	if err != nil {
		return true, err
	}

	// delete the object
	err = ot.Delete(ctx, k)
	if err != nil {
		return true, err
	}

	// update the changelog
	changeBcs, err := t.queueWorldChange(ctx, &WorldChange{
		Key:        key,
		ChangeType: WorldChangeType_WorldChange_OBJECT_DELETE,
	})
	if err != nil {
		return false, err
	}
	// changeBcs may be nil here but this is checked in SetRef.
	changeBcs.SetRef(7, nbcs)

	// success
	return true, nil
}

// _ is a type assertion
var _ world.WorldStateObject = ((*WorldState)(nil))
