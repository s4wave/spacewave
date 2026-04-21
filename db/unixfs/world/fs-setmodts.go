package unixfs_world

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// FsSetModTimestamp sets the modification timestamp of one or more nodes.
func FsSetModTimestamp(
	ctx context.Context,
	obj world.ObjectState,
	sender peer.ID,
	fsType FSType,
	paths [][]string,
	mtime time.Time,
) (rev uint64, sysErr bool, err error) {
	bpaths := unixfs_block.StringSlicesToPaths(paths)
	wOp := NewFsSetModTimestampOp("", fsType, bpaths, mtime)
	return obj.ApplyObjectOp(ctx, wOp, sender)
}

// FsSetModTimestampOpId is the operation id.
var FsSetModTimestampOpId = "hydra/unixfs/set-mod-timestamp"

// NewFsSetModTimestampOp constructs a new FsSetModTimestampOp block.
// repoRef, worktreeArgs can be empty
func NewFsSetModTimestampOp(
	objKey string,
	fsType FSType,
	paths []*unixfs_block.FSPath,
	mtime time.Time,
) *FsSetModTimestampOp {
	return &FsSetModTimestampOp{
		ObjectKey: objKey,
		FsType:    fsType,
		Paths:     paths,
		Timestamp: unixfs_block.ToTimestamp(mtime, true),
	}
}

// NewFsSetModTimestampOpBlock constructs a new FsSetModTimestampOp block.
func NewFsSetModTimestampOpBlock() block.Block {
	return &FsSetModTimestampOp{}
}

// Validate performs cursory checks on the op.
func (o *FsSetModTimestampOp) Validate() error {
	if err := unixfs_block.ValidateSetModTimestamp(o.GetPaths()); err != nil {
		return err
	}
	if err := o.GetFsType().Validate(true); err != nil {
		return err
	}
	// note: allowing empty ts here
	return nil
}

// GetOperationTypeId returns the operation type identifier.
func (o *FsSetModTimestampOp) GetOperationTypeId() string {
	return FsSetModTimestampOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *FsSetModTimestampOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// get the fs object
	obj, err := world.MustGetObject(ctx, worldHandle, o.GetObjectKey())
	if err != nil {
		return false, err
	}

	return o.ApplyWorldObjectOp(ctx, le, obj, sender)
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *FsSetModTimestampOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	// validate
	err = unixfs_block.ValidateSetModTimestamp(o.GetPaths())
	if err != nil {
		return false, err
	}

	_, _, err = world.AccessObjectState(ctx, objectHandle, true, func(bcs *block.Cursor) error {
		switch o.GetFsType() {
		case FSType_FSType_FS_NODE:
			ftree, err := unixfs_block.NewFSTree(ctx, bcs, unixfs_block.NodeType_NodeType_UNKNOWN)
			if err != nil {
				return err
			}
			paths := unixfs_block.PathsToStringSlices(o.GetPaths()...)
			return unixfs_block.SetModTimestamp(ftree, paths, o.GetTimestamp())
		case FSType_FSType_FS_OBJECT:
			return errors.New("TODO apply set-mod-timestamp to fsobject")
		case FSType_FSType_FS_HOST_VOLUME:
			return unixfs_block.ErrCannotModifyHostVolume
		default:
			return errors.Wrap(ErrInvalidFSType, o.GetFsType().String())
		}
	})
	return false, err
}

// MarshalBlock marshals the block to binary.
func (o *FsSetModTimestampOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *FsSetModTimestampOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = ((*FsSetModTimestampOp)(nil))
