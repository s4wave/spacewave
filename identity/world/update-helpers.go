package identity_world

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
)

// storeBlockUpdate stores blk at the object key and applies the update
// operation built from the stored ref. When an object already exists at
// objKey its state is replaced through AccessObjectState; otherwise the
// block is written to a fresh object.
func storeBlockUpdate[T block.Block](
	ctx context.Context,
	w world.WorldState,
	sender peer.ID,
	objKey string,
	blk T,
	newOp func(*bucket.ObjectRef) world.Operation,
) (uint64, bool, error) {
	// Load the existing object state at the key, if any.
	obj, objFound, err := w.GetObject(ctx, objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return 0, false, err
	}

	// Replace the object's block with the update payload.
	setBlock := func(bcs *block.Cursor) error {
		bcs.SetBlock(blk, true)
		bcs.ClearAllRefs()
		return nil
	}

	// Write the block through the existing object or a fresh one.
	var opRef *bucket.ObjectRef
	if objFound {
		var changed bool
		opRef, changed, err = world.AccessObjectState(ctx, obj, false, setBlock)
		if err != nil || !changed {
			return 0, false, err
		}
	} else {
		opRef, err = world.AccessObject(ctx, w.AccessWorldState, nil, setBlock)
		if err != nil {
			return 0, false, err
		}
	}

	// Apply the update operation built from the stored reference.
	op := newOp(opRef)
	return w.ApplyWorldOp(ctx, op, sender)
}

// applyRefUpdate resolves and validates the referenced block through
// resolve, then points the object at objKey at the ref (creating the object
// with its type index when missing). Returns whether the object was newly
// created.
func applyRefUpdate(
	ctx context.Context,
	worldHandle world.WorldState,
	ref *bucket.ObjectRef,
	typeID string,
	resolve func(ctx context.Context) (objKey string, validate func() error, err error),
) (bool, error) {
	// Resolve the target key and validate it before touching the World.
	objKey, validate, err := resolve(ctx)
	if err != nil {
		return false, err
	}
	if err := validate(); err != nil {
		return false, err
	}

	// Point an existing object at the ref or create the object.
	obj, objFound, err := worldHandle.GetObject(ctx, objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return false, err
	}
	if objFound {
		_, err = obj.SetRootRef(ctx, ref)
		return false, err
	}
	{
		// Create the object at the key when it was missing.
		createdObject, err := worldHandle.CreateObject(ctx, objKey, ref)
		world.ReleaseObjectState(createdObject)
		if err != nil {
			return true, err
		}
	}

	// Record the new object's type in its type index.
	if err := world_types.SetObjectType(ctx, worldHandle, objKey, typeID); err != nil {
		return true, err
	}
	return true, nil
}
