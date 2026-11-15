package unixfs_world

import (
	"context"
	"time"

	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// FsSymlink creates a symbolic link from a location to a path.
func FsSymlink(
	ctx context.Context,
	obj world.ObjectState,
	sender peer.ID,
	fsType FSType,
	path []string,
	tgtPath []string,
	tgtIsAbsolute bool,
	ts time.Time,
) (rev uint64, sysErr bool, err error) {
	bpath, tpath := unixfs_block.NewFSPath(path, false), unixfs_block.NewFSPath(tgtPath, tgtIsAbsolute)
	lnk := unixfs_block.NewFSSymlink(tpath)
	wOp := NewFsSymlinkOp("", fsType, bpath, lnk, ts)
	return obj.ApplyObjectOp(ctx, wOp, sender)
}

// NewFsSymlinkOp constructs a new FsSymlinkOp block.
// repoRef and worktreeArgs can be empty.
func NewFsSymlinkOp(
	objKey string,
	fsType FSType,
	path *unixfs_block.FSPath,
	lnk *unixfs_block.FSSymlink,
	ts time.Time,
) *FsSymlinkOp {
	return &FsSymlinkOp{
		ObjectKey: objKey,
		FsType:    fsType,
		Path:      path,
		Symlink:   lnk,
		Timestamp: unixfs_block.ToTimestamp(ts, true),
	}
}

// Validate performs cursory checks on the op.
func (o *FsSymlinkOp) Validate() error {
	if err := unixfs_block.ValidateSymlink(o.GetPath(), o.GetSymlink()); err != nil {
		return err
	}
	if err := o.GetTimestamp().Validate(false); err != nil {
		return err
	}
	if err := o.GetFsType().Validate(true); err != nil {
		return err
	}
	return nil
}

// ApplyWorldOp applies the operation as a world operation.
func (o *FsSymlinkOp) ApplyWorldOp(
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
func (o *FsSymlinkOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	// validate
	err = unixfs_block.ValidateSymlink(o.GetPath(), o.GetSymlink())
	if err != nil {
		return false, world.NewOpRefusedError(err)
	}

	_, _, err = AccessUnixfsObject(ctx, objectHandle, true, o.GetFsType(), func(ftree *unixfs_block.FSTree) error {
		wr := unixfs_block.NewFSWriter(ftree)
		return wr.Symlink(
			ctx,
			o.GetPath().GetNodes(),
			o.GetSymlink().GetTargetPath().GetNodes(),
			o.GetSymlink().GetTargetPath().GetAbsolute(),
			o.GetTimestamp().AsTime(),
		)
	}, nil)

	return false, fsRefusal(err)
}

// _ is a type assertion
var _ world.Operation = (*FsSymlinkOp)(nil)
