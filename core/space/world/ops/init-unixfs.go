package space_world_ops

import (
	"context"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
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

// WriteObjectKeys returns the filesystem object and the FS node type object
// that ApplyWorldOp creates when the World lacks it.
func (o *InitUnixFSOp) WriteObjectKeys() []string {
	return []string{o.GetObjectKey(), world_types.BuildTypeObjectKey(unixfs_world.FSNodeTypeID)}
}

// ApplyWorldOp applies the operation as a world operation.
func (o *InitUnixFSOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// Require an object key for the UnixFS root.
	objKey := o.GetObjectKey()
	if objKey == "" {
		return false, world.ErrEmptyObjectKey
	}

	// Construct and apply a validated filesystem initialization operation.
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

// _ is a type assertion
var _ world.WriteKeysOperation = (*InitUnixFSOp)(nil)
