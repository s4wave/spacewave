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

// ApplyWorldOp applies the operation as a world operation.
func (o *FsSetModTimestampOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// get the fs object
	obj, err := world.MustGetObject(ctx, worldHandle, o.GetObjectKey())
	defer world.ReleaseObjectState(obj)
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
		return false, world.NewOpRefusedError(err)
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
			return world.NewOpRefusedError(errors.New("TODO apply set-mod-timestamp to fsobject"))
		case FSType_FSType_FS_HOST_VOLUME:
			return world.NewOpRefusedError(unixfs_block.ErrCannotModifyHostVolume)
		default:
			return world.NewOpRefusedError(errors.Wrap(ErrInvalidFSType, o.GetFsType().String()))
		}
	})
	return false, fsRefusal(err)
}

// _ is a type assertion
var _ world.Operation = (*FsSetModTimestampOp)(nil)
