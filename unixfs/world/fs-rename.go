package unixfs_world

import (
	"context"
	"time"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/aperturerobotics/hydra/world"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// FsRename renames an inode from one location to another.
// see the FsRename object for details on which fields can be empty.
func FsRename(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	objKey string, fsType FSType,
	srcPath, destPath []string,
	ts time.Time,
) (rev uint64, sysErr bool, err error) {
	bpaths := unixfs_block.StringSlicesToPaths([][]string{srcPath, destPath})

	// perform the fs copy operation
	wOp := NewFsRenameOp(objKey, fsType, bpaths[0], bpaths[1], ts)
	return ws.ApplyWorldOp(ctx, wOp, sender)
}

// FsRenameOpId is the unixfs rename op id.
var FsRenameOpId = "hydra/unixfs/rename"

// NewFsRenameOp constructs a new FsRenameOp block.
// repoRef, worktreeArgs can be empty
func NewFsRenameOp(
	objKey string,
	fsType FSType,
	srcPath, destPath *unixfs_block.FSPath,
	ts time.Time,
) *FsRenameOp {
	return &FsRenameOp{
		ObjectKey: objKey,
		FsType:    fsType,
		SrcPath:   srcPath,
		DestPath:  destPath,
		Timestamp: unixfs_block.ToTimestamp(ts, true),
	}
}

// NewFsRenameOpBlock constructs a new FsRenameOp block.
func NewFsRenameOpBlock() block.Block {
	return &FsRenameOp{}
}

// GetOperationTypeId returns the operation type identifier.
func (o *FsRenameOp) GetOperationTypeId() string {
	return FsRenameOpId
}

// Validate performs cursory checks on the op.
func (o *FsRenameOp) Validate() error {
	if o.GetTimestamp().GetTimeUnixMs() == 0 {
		return unixfs_errors.ErrEmptyTimestamp
	}
	if err := unixfs_block.ValidateRename(o.GetSrcPath(), o.GetDestPath()); err != nil {
		return err
	}
	if o.GetObjectKey() == "" {
		return world.ErrEmptyObjectKey
	}
	if err := o.GetFsType().Validate(true); err != nil {
		return err
	}
	return nil
}

// ApplyWorldOp applies the operation as a world operation.
func (o *FsRenameOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// get the src fs object
	fsObj, err := world.MustGetObject(ctx, worldHandle, o.GetObjectKey())
	if err != nil {
		return false, err
	}

	// TODO: make sure this is not is a cross-fs copy by looking at mountpoints
	return o.ApplyWorldObjectOp(ctx, le, fsObj, sender)
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *FsRenameOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	// validate
	err = unixfs_block.ValidateRename(o.GetSrcPath(), o.GetDestPath())
	if err != nil {
		return false, err
	}

	ts := o.GetTimestamp().ToTime()
	_, _, err = world.AccessObjectState(ctx, objectHandle, true, func(bcs *block.Cursor) error {
		srcFsType := o.GetFsType()
		switch srcFsType {
		case FSType_FSType_FS_NODE:
			ftree, err := unixfs_block.NewFSTree(ctx, bcs, unixfs_block.NodeType_NodeType_UNKNOWN)
			if err != nil {
				return err
			}
			wr := unixfs_block.NewFSWriter(ftree)
			paths := unixfs_block.PathsToStringSlices(o.GetSrcPath(), o.GetDestPath())
			return wr.Rename(ctx, paths[0], paths[1], ts)
		case FSType_FSType_FS_OBJECT:
			return errors.New("TODO rename on fsobject")
		case FSType_FSType_FS_HOST_VOLUME:
			return unixfs_block.ErrCannotModifyHostVolume
		default:
			return errors.Wrap(ErrInvalidFSType, srcFsType.String())
		}
	})
	return false, err
}

// MarshalBlock marshals the block to binary.
func (o *FsRenameOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *FsRenameOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = ((*FsRenameOp)(nil))
