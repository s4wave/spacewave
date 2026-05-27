package space_world_ops

import (
	"context"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/block"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// InitUnixFS initializes an empty UnixFS filesystem in a world.
// Returns any error.
func InitUnixFS(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	objKey string,
	ts time.Time,
) (rev uint64, sysErr bool, err error) {
	op := NewInitUnixFSOp(objKey, ts)
	return ws.ApplyWorldOp(ctx, op, sender)
}

// InitUnixFSOpId is the init UnixFS operation id.
var InitUnixFSOpId = "space/world/init-unixfs"

// NewInitUnixFSOp constructs a new InitUnixFSOp block.
func NewInitUnixFSOp(
	objKey string,
	ts time.Time,
) *InitUnixFSOp {
	return &InitUnixFSOp{
		ObjectKey: objKey,
		Timestamp: timestamp.New(ts),
	}
}

// NewInitUnixFSOpBlock constructs a new InitUnixFSOp block.
func NewInitUnixFSOpBlock() block.Block {
	return &InitUnixFSOp{}
}

// Validate performs cursory checks on the op.
func (o *InitUnixFSOp) Validate() error {
	objKey := o.GetObjectKey()
	if len(objKey) == 0 {
		return world.ErrEmptyObjectKey
	}
	if err := o.GetTimestamp().Validate(false); err != nil {
		return err
	}
	return nil
}

// GetOperationTypeId returns the operation type identifier.
func (o *InitUnixFSOp) GetOperationTypeId() string {
	return InitUnixFSOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *InitUnixFSOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	objKey := o.GetObjectKey()
	if objKey == "" {
		return false, world.ErrEmptyObjectKey
	}

	fsNodeType := unixfs_world.FSType_FSType_FS_NODE
	fsInit := unixfs_world.NewFsInitOp(
		objKey,
		fsNodeType,
		nil,
		false,
		o.GetTimestamp().AsTime(),
	)
	if err := fsInit.Validate(); err != nil {
		return false, err
	}
	sysErr, err = fsInit.ApplyWorldOp(ctx, le, worldHandle, sender)
	if err != nil {
		return sysErr, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *InitUnixFSOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *InitUnixFSOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *InitUnixFSOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupInitUnixFSOp looks up a InitUnixFSOp operation type.
func LookupInitUnixFSOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == InitUnixFSOpId {
		return &InitUnixFSOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ world.Operation = ((*InitUnixFSOp)(nil))
