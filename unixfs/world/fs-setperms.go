package unixfs_world

import (
	"context"
	"io/fs"
	"time"

	"github.com/aperturerobotics/hydra/block"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	"github.com/aperturerobotics/hydra/world"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
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
) error {
	bpaths := unixfs_block.StringSlicesToPaths(paths)
	wOp := NewFsSetPermissionsOp("", fsType, bpaths, perms, ts)
	_, _, err := world.ApplyWaitObjectOp(ctx, obj, wOp, sender)
	return err
}

// FsSetPermissionsOpId is the operation id.
var FsSetPermissionsOpId = "hydra/unixfs/set-permissions"

// NewFsSetPermissionsOp constructs a new FsSetPermissionsOp block.
// repoRef, worktreeArgs can be empty
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

// NewFsSetPermissionsOpBlock constructs a new FsSetPermissionsOp block.
func NewFsSetPermissionsOpBlock() block.Block {
	return &FsSetPermissionsOp{}
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

// GetOperationTypeId returns the operation type identifier.
func (o *FsSetPermissionsOp) GetOperationTypeId() string {
	return FsSetPermissionsOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *FsSetPermissionsOp) ApplyWorldOp(
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
func (o *FsSetPermissionsOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	// validate
	err = unixfs_block.ValidateSetPermissions(o.GetPaths(), o.GetPermissions())
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
			paths := unixfs_block.PathsToStringSlices(o.GetPaths()...)
			return unixfs_block.SetPermissions(ftree, paths, fs.FileMode(o.GetPermissions()), o.GetTimestamp())
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
// This is the initial step of marshaling, before transformations.
func (o *FsSetPermissionsOp) MarshalBlock() ([]byte, error) {
	return proto.Marshal(o)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (o *FsSetPermissionsOp) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, o)
}

// _ is a type assertion
var _ world.Operation = ((*FsSetPermissionsOp)(nil))
