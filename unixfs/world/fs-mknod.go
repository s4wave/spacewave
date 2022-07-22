package unixfs_world

import (
	"context"
	"io/fs"
	"time"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/aperturerobotics/hydra/world"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
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
) error {
	tType := unixfs_block.FSCursorNodeTypeToNodeType(nodeType)
	bpaths := unixfs_block.StringSlicesToPaths(paths)
	// perform the fs init operation
	wOp := NewFsMknodOp("", fsType, bpaths, tType, permissions, ts)
	_, _, err := world.ApplyWaitObjectOp(ctx, obj, wOp, sender)
	return err
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
	if o.GetTimestamp().GetTimeUnixMs() == 0 {
		return unixfs_errors.ErrEmptyTimestamp
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
	obj, err := world.MustGetObject(worldHandle, o.GetObjectKey())
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
			ftree, err := unixfs_block.NewFSTree(bcs, unixfs_block.NodeType_NodeType_UNKNOWN)
			if err != nil {
				return err
			}
			wr := unixfs_block.NewFSWriter(ftree)
			paths := unixfs_block.PathsToStringSlices(o.GetPaths()...)
			nodeType := unixfs_block.NodeTypeToFSCursorNodeType(o.GetNodeType())
			return wr.Mknod(ctx, paths, nodeType, fs.FileMode(o.GetPermissions()), o.GetTimestamp().ToTime())
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
// This is the initial step of marshaling, before transformations.
func (o *FsMknodOp) MarshalBlock() ([]byte, error) {
	return proto.Marshal(o)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (o *FsMknodOp) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, o)
}

// _ is a type assertion
var _ world.Operation = ((*FsMknodOp)(nil))
