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

// FsRemove deletes inodes at paths.
func FsRemove(
	ctx context.Context,
	obj world.ObjectState,
	sender peer.ID,
	fsType FSType,
	paths [][]string,
	ts time.Time,
) error {
	bpaths := unixfs_block.StringSlicesToPaths(paths)
	wOp := NewFsRemoveOp("", fsType, bpaths, ts)
	_, _, err := world.ApplyWaitObjectOp(ctx, obj, wOp, sender)
	return err
}

// FsRemoveOpId is the unixfs remove op id.
var FsRemoveOpId = "hydra/unixfs/remove"

// NewFsRemoveOp constructs a new FsRemoveOp block.
// repoRef, worktreeArgs can be empty
func NewFsRemoveOp(
	objKey string,
	fsType FSType,
	paths []*unixfs_block.FSPath,
	ts time.Time,
) *FsRemoveOp {
	return &FsRemoveOp{
		ObjectKey: objKey,
		FsType:    fsType,
		Paths:     paths,
		Timestamp: unixfs_block.ToTimestamp(ts, true),
	}
}

// NewFsRemoveOpBlock constructs a new FsRemoveOp block.
func NewFsRemoveOpBlock() block.Block {
	return &FsRemoveOp{}
}

// GetOperationTypeId returns the operation type identifier.
func (o *FsRemoveOp) GetOperationTypeId() string {
	return FsRemoveOpId
}

// Validate performs cursory checks on the op.
func (o *FsRemoveOp) Validate() error {
	if o.GetTimestamp().GetTimeUnixMs() == 0 {
		return unixfs_errors.ErrEmptyTimestamp
	}
	if err := unixfs_block.ValidateRemove(o.GetPaths()); err != nil {
		return err
	}
	if err := o.GetFsType().Validate(true); err != nil {
		return err
	}
	return nil
}

// ApplyWorldOp applies the operation as a world operation.
func (o *FsRemoveOp) ApplyWorldOp(
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
func (o *FsRemoveOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	// validate
	err = unixfs_block.ValidateRemove(o.GetPaths())
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
			wr := unixfs_block.NewFSWriter(ftree)
			paths := unixfs_block.PathsToStringSlices(o.GetPaths()...)
			return wr.Remove(ctx, paths, o.GetTimestamp().ToTime())
		case FSType_FSType_FS_OBJECT:
			return errors.New("TODO remove from fsobject")
		case FSType_FSType_FS_HOST_VOLUME:
			return unixfs_block.ErrCannotModifyHostVolume
		default:
			return errors.Wrap(ErrInvalidFSType, o.GetFsType().String())
		}
	})
	return false, err
}

// MarshalBlock marshals the block to binary.
func (o *FsRemoveOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *FsRemoveOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = ((*FsRemoveOp)(nil))
