package bstore_world

import (
	"context"

	"github.com/s4wave/spacewave/core/bstore"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// NewUpdateBlockStoreStateOp constructs a new UpdateBlockStoreStateOp block.
func NewUpdateBlockStoreStateOp(updatedState *BlockStoreState, ifNotExists bool) *UpdateBlockStoreStateOp {
	return &UpdateBlockStoreStateOp{
		UpdatedState: updatedState,
		IfNotExists:  ifNotExists,
	}
}

// Validate performs cursory validation of the operation.
// Should not block.
func (o *UpdateBlockStoreStateOp) Validate() error {
	if err := o.GetUpdatedState().Validate(); err != nil {
		return err
	}
	return nil
}

// ApplyWorldOp applies the operation as a world operation.
func (o *UpdateBlockStoreStateOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// Compute the World object key for the updated block store state.
	bstoreRef := o.GetUpdatedState().GetRef()
	objKey := NewBlockStoreStateKey(
		bstoreRef.GetProviderResourceRef().GetProviderId(),
		bstoreRef.GetProviderResourceRef().GetProviderAccountId(),
		bstoreRef.GetProviderResourceRef().GetId(),
	)

	// Refuse to overwrite an existing block store state when requested.
	if o.GetIfNotExists() {
		objectState, exists, err := worldHandle.GetObject(ctx, objKey)
		world.ReleaseObjectState(objectState)
		if err != nil {
			return false, err
		}
		if exists {
			return false, world.NewOpRefusedError(bstore.ErrBlockStoreExists)
		}
	}

	// Update the stored block store state block when it differs.
	_, _, err = world.AccessWorldObject(ctx, worldHandle, objKey, true, func(bcs *block.Cursor) error {
		storedInfo, err := UnmarshalBlockStoreState(ctx, bcs)
		if err != nil {
			return err
		}
		if !storedInfo.EqualVT(o.GetUpdatedState()) {
			bcs.SetBlock(o.GetUpdatedState().CloneVT(), true)
		}
		return nil
	})
	if err != nil {
		return false, err
	}

	// set the object type if necessary
	err = world_types.SetObjectType(ctx, worldHandle, objKey, BlockStoreStateTypeID)
	if err != nil {
		return false, err
	}

	return false, nil
}

// _ is a type assertion
var _ world.Operation = (*UpdateBlockStoreStateOp)(nil)
