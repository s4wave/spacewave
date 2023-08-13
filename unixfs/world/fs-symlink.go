package unixfs_world

import (
	"context"
	"time"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/aperturerobotics/hydra/world"
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
	ts time.Time,
) (rev uint64, sysErr bool, err error) {
	bpath, tpath := unixfs_block.NewFSPath(path), unixfs_block.NewFSPath(tgtPath)
	lnk := unixfs_block.NewFSSymlink(tpath)
	wOp := NewFsSymlinkOp("", fsType, bpath, lnk, ts)
	return world.ApplyWaitObjectOp(ctx, obj, wOp, sender)
}

// FsSymlinkOpId is the operation id.
var FsSymlinkOpId = "hydra/unixfs/symlink"

// NewFsSymlinkOp constructs a new FsSymlinkOp block.
// repoRef, worktreeArgs can be empty
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

// NewFsSymlinkOpBlock constructs a new FsSymlinkOp block.
func NewFsSymlinkOpBlock() block.Block {
	return &FsSymlinkOp{}
}

// Validate performs cursory checks on the op.
func (o *FsSymlinkOp) Validate() error {
	if err := unixfs_block.ValidateSymlink(o.GetPath(), o.GetSymlink()); err != nil {
		return err
	}
	if o.GetTimestamp().GetTimeUnixMs() == 0 {
		return unixfs_errors.ErrEmptyTimestamp
	}
	if err := o.GetFsType().Validate(true); err != nil {
		return err
	}
	return nil
}

// GetOperationTypeId returns the operation type identifier.
func (o *FsSymlinkOp) GetOperationTypeId() string {
	return FsSymlinkOpId
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
		return false, err
	}

	_, _, err = AccessUnixfsObject(ctx, objectHandle, true, o.GetFsType(), func(ftree *unixfs_block.FSTree) error {
		wr := unixfs_block.NewFSWriter(ftree)
		return wr.Symlink(ctx, o.GetPath().GetNodes(), o.GetSymlink().GetTargetPath().GetNodes(), o.GetTimestamp().ToTime())
	}, nil)

	return false, err
}

// MarshalBlock marshals the block to binary.
func (o *FsSymlinkOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *FsSymlinkOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = ((*FsSymlinkOp)(nil))
