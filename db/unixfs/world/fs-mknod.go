package unixfs_world

import (
	"context"
	"io/fs"
	"time"

	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// FsMknod creates a inode at the given paths.
func FsMknod(
	ctx context.Context,
	obj world.ObjectState,
	sender peer.ID,
	fsType FSType,
	paths [][]string,
	nodeType unixfs.FSCursorNodeType,
	permissions fs.FileMode,
	ts time.Time,
) (rev uint64, sysErr bool, err error) {
	tType := unixfs_block.FSCursorNodeTypeToNodeType(nodeType)
	bpaths := unixfs_block.StringSlicesToPaths(paths)
	wOp := NewFsMknodOp("", fsType, bpaths, tType, permissions, ts)
	return obj.ApplyObjectOp(ctx, wOp, sender)
}

// FsMknodOpId is the operation id.
var FsMknodOpId = "hydra/unixfs/mknod"

// NewFsMknodOp constructs a new FsMknodOp block.
// repoRef, worktreeArgs can be empty
func NewFsMknodOp(
	objKey string,
	fsType FSType,
	paths []*unixfs_block.FSPath,
	nodeType unixfs_block.NodeType,
	permissions fs.FileMode,
	ts time.Time,
) *FsMknodOp {
	return &FsMknodOp{
		ObjectKey:   objKey,
		FsType:      fsType,
		Paths:       paths,
		NodeType:    nodeType,
		Timestamp:   unixfs_block.ToTimestamp(ts, true),
		Permissions: uint32(permissions.Perm()),
	}
}

// NewFsMknodOpBlock constructs a new FsMknodOp block.
func NewFsMknodOpBlock() block.Block {
	return &FsMknodOp{}
}

// Validate performs cursory checks on the op.
func (o *FsMknodOp) Validate() error {
	if err := unixfs_block.ValidateMknod(o.GetPaths(), o.GetNodeType()); err != nil {
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

// GetOperationTypeId returns the operation type identifier.
func (o *FsMknodOp) GetOperationTypeId() string {
	return FsMknodOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *FsMknodOp) ApplyWorldOp(
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
func (o *FsMknodOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	// validate
	err = unixfs_block.ValidateMknod(o.GetPaths(), o.GetNodeType())
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
			nodeType := unixfs_block.NodeTypeToFSCursorNodeType(o.GetNodeType())
			return wr.Mknod(ctx, paths, nodeType, fs.FileMode(o.GetPermissions()), o.GetTimestamp().AsTime())
		case FSType_FSType_FS_OBJECT:
			return errors.New("TODO apply mknod to fsobject")
		case FSType_FSType_FS_HOST_VOLUME:
			return unixfs_block.ErrCannotModifyHostVolume
		default:
			return errors.Wrap(ErrInvalidFSType, o.GetFsType().String())
		}
	})
	return false, err
}

// MarshalBlock marshals the block to binary.
func (o *FsMknodOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *FsMknodOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = ((*FsMknodOp)(nil))
