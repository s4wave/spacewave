package unixfs_world

import (
	"context"
	"io/fs"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// FsSetPermissions sets the permissions of one or more nodes.
func FsSetPermissions(
	ctx context.Context,
	obj world.ObjectState,
	sender peer.ID,
	fsType FSType,
	paths [][]string,
	perms fs.FileMode,
	ts time.Time,
) (rev uint64, sysErr bool, err error) {
	bpaths := unixfs_block.StringSlicesToPaths(paths)
	wOp := NewFsSetPermissionsOp("", fsType, bpaths, perms, ts)
	return obj.ApplyObjectOp(ctx, wOp, sender)
}

// NewFsSetPermissionsOp constructs a new FsSetPermissionsOp block.
// repoRef and worktreeArgs can be empty.
func NewFsSetPermissionsOp(
	objKey string,
	fsType FSType,
	paths []*unixfs_block.FSPath,
	perms fs.FileMode,
	ts time.Time,
) *FsSetPermissionsOp {
	return &FsSetPermissionsOp{
		ObjectKey:   objKey,
		FsType:      fsType,
		Paths:       paths,
		Permissions: uint32(perms.Perm()),
		Timestamp:   unixfs_block.ToTimestamp(ts, true),
	}
}

// Validate performs cursory checks on the op.
func (o *FsSetPermissionsOp) Validate() error {
	if err := unixfs_block.ValidateSetPermissions(o.GetPaths(), o.GetPermissions()); err != nil {
		return err
	}
	if err := o.GetFsType().Validate(true); err != nil {
		return err
	}
	// note: allowing empty ts here
	return nil
}

// ApplyWorldOp applies the operation as a world operation.
func (o *FsSetPermissionsOp) ApplyWorldOp(
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
func (o *FsSetPermissionsOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	// validate
	err = unixfs_block.ValidateSetPermissions(o.GetPaths(), o.GetPermissions())
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
			return unixfs_block.SetPermissions(ftree, paths, fs.FileMode(o.GetPermissions()), o.GetTimestamp())
		case FSType_FSType_FS_OBJECT:
			return world.NewOpRefusedError(errors.New("TODO apply set-permissions to fsobject"))
		case FSType_FSType_FS_HOST_VOLUME:
			return world.NewOpRefusedError(unixfs_block.ErrCannotModifyHostVolume)
		default:
			return world.NewOpRefusedError(errors.Wrap(ErrInvalidFSType, o.GetFsType().String()))
		}
	})
	return false, fsRefusal(err)
}

// _ is a type assertion
var _ world.Operation = (*FsSetPermissionsOp)(nil)
