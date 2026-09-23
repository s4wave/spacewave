package space_world_ops

import (
	"context"

	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// SetSpaceIndexPathOpID identifies the atomic default-object operation.
const SetSpaceIndexPathOpID = "space/world/set-index-path"

// Validate checks the operation timestamp.
func (o *SetSpaceIndexPathOp) Validate() error {
	return o.GetTimestamp().Validate(false)
}

// GetOperationTypeId returns the operation identifier.
func (o *SetSpaceIndexPathOp) GetOperationTypeId() string {
	return SetSpaceIndexPathOpID
}

// ApplyWorldOp changes only the index path in the transaction's current settings.
// Conditional repairs preserve a path another operation has already selected.
func (o *SetSpaceIndexPathOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (bool, error) {
	current, err := space_world.LookupSpaceSettingsBody(ctx, ws)
	if err != nil {
		return false, err
	}
	if o.ExpectedIndexPath != nil && current.GetIndexPath() != o.GetExpectedIndexPath() {
		return false, nil
	}

	settings := &space_world.SpaceSettings{}
	if current != nil {
		settings = current.CloneVT()
	}
	settings.IndexPath = o.GetIndexPath()
	update := &SetSpaceSettingsOp{
		Settings:  settings,
		Overwrite: true,
		Timestamp: o.GetTimestamp(),
	}
	return update.ApplyWorldOp(ctx, le, ws, sender)
}

// ApplyWorldObjectOp is unsupported; the operation addresses the World's settings.
func (o *SetSpaceIndexPathOp) ApplyWorldObjectOp(
	context.Context,
	*logrus.Entry,
	world.ObjectState,
	peer.ID,
) (bool, error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock encodes the operation.
func (o *SetSpaceIndexPathOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock decodes the operation.
func (o *SetSpaceIndexPathOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

var _ world.Operation = (*SetSpaceIndexPathOp)(nil)
