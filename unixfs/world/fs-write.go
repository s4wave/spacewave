package unixfs_world

import (
	"context"
	"time"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/blob"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/aperturerobotics/hydra/world"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// FsWrite writes to a file at the given location.
func FsWrite(
	ctx context.Context,
	obj world.ObjectState,
	sender peer.ID,
	fsType FSType,
	path []string,
	offset int64,
	data []byte,
	ts time.Time,
) error {
	fpath := unixfs_block.NewFSPath(path)
	// writes to the blb object
	blbObjRef, err := world.AccessObject(ctx, obj.AccessWorldState, nil, func(bcs *block.Cursor) error {
		bcs.SetRefAtCursor(nil, true)
		_, err := blob.BuildBlobWithBytes(ctx, data, bcs)
		return err
	})
	if err != nil {
		return err
	}
	wOp := NewFsWriteOp("", fsType, fpath, offset, blbObjRef.GetRootRef(), ts)
	_, _, err = world.ApplyWaitObjectOp(ctx, obj, wOp, sender)
	return err
}

// FsWriteOpId is the operation id.
var FsWriteOpId = "hydra/unixfs/write"

// NewFsWriteOp constructs a new FsWriteOp block.
// repoRef, worktreeArgs can be empty
func NewFsWriteOp(
	objKey string,
	fsType FSType,
	path *unixfs_block.FSPath,
	offset int64,
	blbRef *block.BlockRef,
	ts time.Time,
) *FsWriteOp {
	return &FsWriteOp{
		ObjectKey: objKey,
		FsType:    fsType,
		Path:      path,
		Offset:    offset,
		BlobRef:   blbRef,
		Timestamp: unixfs_block.ToTimestamp(ts, true),
	}
}

// NewFsWriteOpBlock constructs a new FsWriteOp block.
func NewFsWriteOpBlock() block.Block {
	return &FsWriteOp{}
}

// Validate performs cursory checks on the op.
func (o *FsWriteOp) Validate() error {
	if err := o.GetFsType().Validate(true); err != nil {
		return err
	}
	if o.GetTimestamp().GetTimeUnixMs() == 0 {
		return unixfs_errors.ErrEmptyTimestamp
	}
	if err := o.GetBlobRef().Validate(); err != nil {
		return err
	}
	return nil
}

// GetOperationTypeId returns the operation type identifier.
func (o *FsWriteOp) GetOperationTypeId() string {
	return FsWriteOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *FsWriteOp) ApplyWorldOp(
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
func (o *FsWriteOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	// validate
	err = unixfs_block.ValidateWrite(o.GetPath(), o.GetOffset())
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

			fpath := o.GetPath().GetNodes()
			fullValidate := true
			forceUseBlob := false

			return unixfs_block.WriteBlob(
				ctx,
				ftree,
				fpath,
				o.GetOffset(),
				o.GetBlobRef(),
				fullValidate,
				forceUseBlob,
				o.GetTimestamp(),
			)
		case FSType_FSType_FS_OBJECT:
			return errors.New("TODO apply write to fsobject")
		case FSType_FSType_FS_HOST_VOLUME:
			return unixfs_block.ErrCannotModifyHostVolume
		default:
			return errors.Wrap(ErrInvalidFSType, o.GetFsType().String())
		}
	})
	return false, err
}

// MarshalBlock marshals the block to binary.
func (o *FsWriteOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *FsWriteOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = ((*FsWriteOp)(nil))
