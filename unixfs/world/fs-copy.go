package unixfs_world

import (
	"context"
	"time"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	"github.com/aperturerobotics/hydra/world"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// FsCopy copies an inode from one location to another.
// see the FsCopy object for details on which fields can be empty.
// NOTE: we use a World Op here to enable efficient cross-fs renames.
func FsCopy(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	objKey string, fsType FSType,
	srcPath, destPath []string,
	ts time.Time,
) (rev uint64, sysErr bool, err error) {
	// perform the fs copy operation
	bpaths := unixfs_block.StringSlicesToPaths([][]string{srcPath, destPath})
	wOp := NewFsCopyOp(objKey, fsType, bpaths[0], bpaths[1], ts)

	// NOTE: we must return the object rev, not the world rev here.
	_, sysErr, err = ws.ApplyWorldOp(ctx, wOp, sender)
	if err != nil {
		return 0, sysErr, err
	}
	obj, err := world.MustGetObject(ctx, ws, objKey)
	if err != nil {
		return 0, false, err
	}
	_, rev, err = obj.GetRootRef(ctx)
	if err != nil {
		return 0, false, err
	}
	return rev, false, nil
}

// FsCopyOpId is the unixfs copy op id.
var FsCopyOpId = "hydra/unixfs/copy"

// NewFsCopyOp constructs a new FsCopyOp block.
// repoRef, worktreeArgs can be empty
func NewFsCopyOp(
	objKey string,
	fsType FSType,
	srcPath, destPath *unixfs_block.FSPath,
	ts time.Time,
) *FsCopyOp {
	return &FsCopyOp{
		ObjectKey: objKey,
		FsType:    fsType,
		SrcPath:   srcPath,
		DestPath:  destPath,
		Timestamp: unixfs_block.ToTimestamp(ts, true),
	}
}

// NewFsCopyOpBlock constructs a new FsCopyOp block.
func NewFsCopyOpBlock() block.Block {
	return &FsCopyOp{}
}

// GetOperationTypeId returns the operation type identifier.
func (o *FsCopyOp) GetOperationTypeId() string {
	return FsCopyOpId
}

// Validate performs cursory checks on the op.
func (o *FsCopyOp) Validate() error {
	if err := o.GetTimestamp().Validate(false); err != nil {
		return err
	}
	if err := unixfs_block.ValidateCopy(o.GetSrcPath(), o.GetDestPath()); err != nil {
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
func (o *FsCopyOp) ApplyWorldOp(
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
func (o *FsCopyOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	// validate
	err = unixfs_block.ValidateCopy(o.GetSrcPath(), o.GetDestPath())
	if err != nil {
		return false, err
	}

	ts := o.GetTimestamp().AsTime()
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
			return wr.Copy(ctx, paths[0], paths[1], ts)
		case FSType_FSType_FS_OBJECT:
			return errors.New("TODO copy on fsobject")
		case FSType_FSType_FS_HOST_VOLUME:
			return unixfs_block.ErrCannotModifyHostVolume
		default:
			return errors.Wrap(ErrInvalidFSType, srcFsType.String())
		}
	})
	return false, err
}

// MarshalBlock marshals the block to binary.
func (o *FsCopyOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *FsCopyOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = ((*FsCopyOp)(nil))
