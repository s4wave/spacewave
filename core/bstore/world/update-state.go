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

// UpdateBlockStoreStateOpId is the operation to update information about a block store.
var UpdateBlockStoreStateOpId = BlockStoreStateTypeID + "/update"

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

// GetOperationTypeId returns the operation type identifier.
func (o *UpdateBlockStoreStateOp) GetOperationTypeId() string {
	return UpdateBlockStoreStateOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *UpdateBlockStoreStateOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	bstoreRef := o.GetUpdatedState().GetRef()
	objKey := NewBlockStoreStateKey(
		bstoreRef.GetProviderResourceRef().GetProviderId(),
		bstoreRef.GetProviderResourceRef().GetProviderAccountId(),
		bstoreRef.GetProviderResourceRef().GetId(),
	)
	if o.GetIfNotExists() {
		_, exists, err := worldHandle.GetObject(ctx, objKey)
		if err != nil {
			return false, err
		}
		if exists {
			return false, bstore.ErrBlockStoreExists
		}
	}

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

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *UpdateBlockStoreStateOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (o *UpdateBlockStoreStateOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (o *UpdateBlockStoreStateOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = ((*UpdateBlockStoreStateOp)(nil))
