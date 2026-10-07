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
	s4wave_command "github.com/s4wave/spacewave/sdk/command"
	"github.com/sirupsen/logrus"
)

// ErrInvalidSettings is returned if the settings are invalid.
var ErrInvalidSettings = errors.New("settings cannot be nil")

// ErrChangelogUnsupported is returned when the settings enable a changelog in
// a World that cannot keep one.
var ErrChangelogUnsupported = errors.New("world cannot keep a changelog")

// SetSpaceSettings sets the space settings in a world and applies their
// changelog setting to it.
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
	// Select the replacement settings and resolve the target object key.
	settings := o.GetSettings()
	objKey := o.GetObjectKey()
	if objKey == "" {
		objKey = DefaultSpaceSettingsObjectKey
	}

	// Refuse to replace existing settings unless the op overwrites them.
	if !o.GetOverwrite() {
		objectState, exists, err := worldHandle.GetObject(ctx, objKey)
		world.ReleaseObjectState(objectState)
		if err != nil {
			return false, err
		}
		if exists {
			return false, world.ErrObjectExists
		}
	}

	// Merge keybinding overrides when the operation includes an expected set.
	if o.GetExpectedKeybindingOverrides() != nil {
		current, err := space_world.LookupSpaceSettingsBody(ctx, worldHandle)
		if err != nil {
			return false, err
		}
		settings, err = o.mergeKeybindingSettings(current, settings)
		if err != nil {
			return false, err
		}
	}

	// Write the settings to the object, creating it when missing.
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

	// Mark the object as SpaceSettings.
	spaceSettingsTypeID := space_world.SpaceSettingsBlockType.GetBlockTypeID()
	if err := world_types.SetObjectType(ctx, worldHandle, objKey, spaceSettingsTypeID); err != nil {
		return false, err
	}

	// Apply the Space's changelog setting to the World in the same transaction.
	if objKey != space_world.SpaceSettingsObjectKey {
		return false, nil
	}
	return false, applySpaceChangelog(ctx, worldHandle, settings.GetChangelogEnabled())
}

// changelogWorld is a World whose changelog can be switched on or off.
type changelogWorld interface {
	// SetChangelogDisabled switches the changelog and reports a change.
	SetChangelogDisabled(ctx context.Context, disable bool) (bool, error)
}

// applySpaceChangelog applies a Space's changelog setting to its World.
// A World that keeps no changelog satisfies only a disabled setting.
func applySpaceChangelog(ctx context.Context, ws world.WorldState, enabled bool) error {
	cw, ok := ws.(changelogWorld)
	if !ok {
		if enabled {
			return ErrChangelogUnsupported
		}
		return nil
	}
	_, err := cw.SetChangelogDisabled(ctx, !enabled)
	return err
}

// ApplyWorldObjectOp applies the operation to a world object handle.
// An object handle cannot reach the World changelog, so the stored
// changelog setting is kept.
func (o *SetSpaceSettingsOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	// Require replacement settings before updating the object body.
	settings := o.GetSettings()
	if settings == nil {
		return false, ErrInvalidSettings
	}

	// Write the settings to the object, keeping its changelog setting.
	_, _, err = world.AccessObjectState(ctx, objectHandle, true, func(bcs *block.Cursor) error {
		// Read the stored settings, which may be missing.
		currentBlock, err := bcs.Unmarshal(ctx, space_world.NewSpaceSettingsBlock)
		if err != nil {
			return err
		}
		current, valid := currentBlock.(*space_world.SpaceSettings)
		if currentBlock != nil && !valid {
			return ErrInvalidSettings
		}

		// Build the replacement, merging keybindings when the op expects them.
		next := settings.CloneVT()
		if o.GetExpectedKeybindingOverrides() != nil {
			next, err = o.mergeKeybindingSettings(current, settings)
			if err != nil {
				return err
			}
		}

		// Keep the stored changelog setting and write the result.
		next.ChangelogEnabled = current.GetChangelogEnabled()
		bcs.SetBlock(next, true)
		return nil
	})
	return false, err
}

func (o *SetSpaceSettingsOp) mergeKeybindingSettings(
	current, replacement *space_world.SpaceSettings,
) (*space_world.SpaceSettings, error) {
	// Merge the expected keybinding edits into the current space settings.
	if current == nil {
		current = &space_world.SpaceSettings{}
	}
	merged, err := s4wave_command.MergeKeybindingOverrideSet(
		current.GetKeybindingOverrides(),
		o.GetExpectedKeybindingOverrides(),
		replacement.GetKeybindingOverrides(),
	)
	if err != nil {
		return nil, err
	}
	next := current.CloneVT()
	next.KeybindingOverrides = merged
	return next, nil
}

// MarshalBlock marshals the block to binary.
func (o *SetSpaceSettingsOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *SetSpaceSettingsOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupSetSpaceSettingsOp looks up Space settings operations.
func LookupSetSpaceSettingsOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	switch operationTypeID {
	case SetSpaceSettingsOpId:
		return &SetSpaceSettingsOp{}, nil
	case SetSpaceIndexPathOpID:
		return &SetSpaceIndexPathOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ world.Operation = (*SetSpaceSettingsOp)(nil)
