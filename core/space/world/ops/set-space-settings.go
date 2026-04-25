package space_world_ops

import (
	"context"
	"errors"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// ErrInvalidSettings is returned if the settings are invalid.
var ErrInvalidSettings = errors.New("settings cannot be nil")

// SetSpaceSettings sets the space settings in a world.
// Returns any error.
func SetSpaceSettings(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	objKey string,
	settings *space_world.SpaceSettings,
	overwrite bool,
	ts time.Time,
) (rev uint64, sysErr bool, err error) {
	op := NewSetSpaceSettingsOp(
		objKey,
		settings,
		overwrite,
		ts,
	)
	return ws.ApplyWorldOp(ctx, op, sender)
}

// SetSpaceSettingsOpId is the space settings init operation id.
var SetSpaceSettingsOpId = "space/world/set-settings"

// DefaultSpaceSettingsObjectKey is the default object key for space settings.
const DefaultSpaceSettingsObjectKey = "settings"

// NewSetSpaceSettingsOp constructs a new SetSpaceSettingsOp block.
func NewSetSpaceSettingsOp(
	objKey string,
	settings *space_world.SpaceSettings,
	overwrite bool,
	ts time.Time,
) *SetSpaceSettingsOp {
	if objKey == "" {
		objKey = DefaultSpaceSettingsObjectKey
	}
	return &SetSpaceSettingsOp{
		ObjectKey: objKey,
		Settings:  settings,
		Overwrite: overwrite,
		Timestamp: timestamp.New(ts),
	}
}

// NewSetSpaceSettingsOpBlock constructs a new SetSpaceSettingsOp block.
func NewSetSpaceSettingsOpBlock() block.Block {
	return &SetSpaceSettingsOp{}
}

// Validate performs cursory checks on the op.
func (o *SetSpaceSettingsOp) Validate() error {
	objKey := o.GetObjectKey()
	if objKey == "" {
		objKey = DefaultSpaceSettingsObjectKey
	}
	if len(objKey) == 0 {
		return world.ErrEmptyObjectKey
	}
	if err := o.GetTimestamp().Validate(false); err != nil {
		return err
	}
	if o.GetSettings() == nil {
		return ErrInvalidSettings
	}
	return nil
}

// GetOperationTypeId returns the operation type identifier.
func (o *SetSpaceSettingsOp) GetOperationTypeId() string {
	return SetSpaceSettingsOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *SetSpaceSettingsOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	settings := o.GetSettings()
	objKey := o.GetObjectKey()
	if objKey == "" {
		objKey = DefaultSpaceSettingsObjectKey
	}

	// check if exists if we need to prevent overwriting
	if !o.GetOverwrite() {
		_, exists, err := worldHandle.GetObject(ctx, objKey)
		if err != nil {
			return false, err
		}
		if exists {
			return false, world.ErrObjectExists
		}
	}

	// write the settings to the object (creating it if it doesnt exist)
	_, _, err = world.AccessWorldObject(
		ctx,
		worldHandle,
		objKey,
		true,
		func(bcs *block.Cursor) error {
			bcs.SetBlock(settings.CloneVT(), true)
			return nil
		},
	)
	if err != nil {
		return false, err
	}

	// set the object type
	spaceSettingsTypeID := space_world.SpaceSettingsBlockType.GetBlockTypeID()
	if err := world_types.SetObjectType(ctx, worldHandle, objKey, spaceSettingsTypeID); err != nil {
		return false, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *SetSpaceSettingsOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	settings := o.GetSettings()
	if settings == nil {
		return false, ErrInvalidSettings
	}

	// write the settings to the object
	_, _, err = world.AccessObjectState(ctx, objectHandle, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(settings.CloneVT(), true)
		return nil
	})
	if err != nil {
		return false, err
	}

	return false, nil
}

// MarshalBlock marshals the block to binary.
func (o *SetSpaceSettingsOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *SetSpaceSettingsOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupSetSpaceSettingsOp looks up a SetSpaceSettingsOp operation type.
func LookupSetSpaceSettingsOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == SetSpaceSettingsOpId {
		return &SetSpaceSettingsOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ world.Operation = ((*SetSpaceSettingsOp)(nil))
