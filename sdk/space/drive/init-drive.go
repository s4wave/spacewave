package s4wave_drive

import (
	"context"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// InitDriveOpId is the operation ID for InitDriveOp.
const InitDriveOpId = "space/world/init-drive"

// InitDrive initializes a Drive world object.
func InitDrive(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	objKey string,
	displayName string,
	roots []*DriveRoot,
	ts time.Time,
) (rev uint64, sysErr bool, err error) {
	op := NewInitDriveOp(objKey, displayName, roots, ts)
	return ws.ApplyWorldOp(ctx, op, sender)
}

// NewInitDriveOp constructs an InitDriveOp.
func NewInitDriveOp(
	objKey string,
	displayName string,
	roots []*DriveRoot,
	ts time.Time,
) *InitDriveOp {
	return &InitDriveOp{
		ObjectKey:   objKey,
		DisplayName: displayName,
		Roots:       roots,
		Timestamp:   timestamp.New(ts),
	}
}

// NewInitDriveOpBlock constructs an InitDriveOp block.
func NewInitDriveOpBlock() block.Block {
	return &InitDriveOp{}
}

// Validate performs cursory checks on the op.
func (o *InitDriveOp) Validate() error {
	if o.GetObjectKey() == "" {
		return errors.Wrap(world.ErrEmptyObjectKey, "object_key")
	}
	if o.GetTimestamp() == nil {
		return errors.New("timestamp is required")
	}
	if err := o.GetTimestamp().Validate(false); err != nil {
		return err
	}
	if len(o.GetRoots()) == 0 {
		return errors.New("at least one root is required")
	}
	for _, root := range o.GetRoots() {
		if root.GetRootId() == "" {
			return errors.New("root_id is required")
		}
		if root.GetRootObjectKey() == "" {
			return errors.New("root_object_key is required")
		}
		if root.GetRootType() == "" {
			return errors.New("root_type is required")
		}
	}
	return nil
}

// GetOperationTypeId returns the operation type identifier.
func (o *InitDriveOp) GetOperationTypeId() string {
	return InitDriveOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *InitDriveOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	if err := o.Validate(); err != nil {
		return false, err
	}

	for _, root := range o.GetRoots() {
		if root.GetRootType() != unixfs_world.FSNodeTypeID {
			return false, errors.Errorf("drive root %s: unsupported root type %q", root.GetRootId(), root.GetRootType())
		}
		if err := world_types.CheckObjectType(ctx, worldHandle, root.GetRootObjectKey(), unixfs_world.FSNodeTypeID); err != nil {
			return false, err
		}
	}

	ts := o.GetTimestamp()
	roots := make([]*DriveRoot, 0, len(o.GetRoots()))
	for _, root := range o.GetRoots() {
		next := root.CloneVT()
		if next.AddedAt == nil {
			next.AddedAt = ts.CloneVT()
		}
		roots = append(roots, next)
	}

	state := &Drive{
		DisplayName: o.GetDisplayName(),
		Roots:       roots,
		CreatedAt:   ts.CloneVT(),
		UpdatedAt:   ts.CloneVT(),
	}
	_, _, err = world.CreateWorldObject(ctx, worldHandle, o.GetObjectKey(), func(bcs *block.Cursor) error {
		bcs.SetBlock(state, true)
		return nil
	})
	if err != nil {
		return false, err
	}

	if err := world_types.SetObjectType(ctx, worldHandle, o.GetObjectKey(), DriveTypeID); err != nil {
		return false, err
	}
	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *InitDriveOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *InitDriveOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block from binary.
func (o *InitDriveOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupInitDriveOp looks up an InitDriveOp operation type.
func LookupInitDriveOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == InitDriveOpId {
		return &InitDriveOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ world.Operation = (*InitDriveOp)(nil)
