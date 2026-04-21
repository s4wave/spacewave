package unixfs_world

import (
	"context"
	"time"

	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// FsWriteAt writes to a file at the given location.
func FsWriteAt(
	ctx context.Context,
	obj world.ObjectState,
	sender peer.ID,
	fsType FSType,
	path []string,
	offset int64,
	data []byte,
	ts time.Time,
) (rev uint64, sysErr bool, err error) {
	// Build the blob.
	fpath := unixfs_block.NewFSPath(path, false)
	blbObjRef, err := world.AccessObject(
		ctx,
		obj.AccessWorldState,
		nil,
		func(bcs *block.Cursor) error {
			bcs.SetRefAtCursor(nil, true)
			_, err := blob.BuildBlobWithBytes(ctx, data, bcs)
			return err
		},
	)
	if err != nil {
		return 0, true, err
	}

	// Transmit the blob in a fs write operation.
	wOp := NewFsWriteAtOp("", fsType, fpath, offset, blbObjRef.GetRootRef(), ts)
	return obj.ApplyObjectOp(ctx, wOp, sender)
}

// FsWriteAtOpId is the operation id.
var FsWriteAtOpId = "hydra/unixfs/write-at"

// NewFsWriteAtOp constructs a new FsWriteAtOp block.
// repoRef, worktreeArgs can be empty
func NewFsWriteAtOp(
	objKey string,
	fsType FSType,
	path *unixfs_block.FSPath,
	offset int64,
	blbRef *block.BlockRef,
	ts time.Time,
) *FsWriteAtOp {
	return &FsWriteAtOp{
		ObjectKey: objKey,
		FsType:    fsType,
		Path:      path,
		Offset:    offset,
		BlobRef:   blbRef,
		Timestamp: unixfs_block.ToTimestamp(ts, true),
	}
}

// NewFsWriteAtOpBlock constructs a new FsWriteAtOp block.
func NewFsWriteAtOpBlock() block.Block {
	return &FsWriteAtOp{}
}

// Validate performs cursory checks on the op.
func (o *FsWriteAtOp) Validate() error {
	if err := o.GetFsType().Validate(true); err != nil {
		return err
	}
	if err := o.GetTimestamp().Validate(false); err != nil {
		return err
	}
	// disallow empty blob ref
	if err := o.GetBlobRef().Validate(false); err != nil {
		return err
	}
	return nil
}

// GetOperationTypeId returns the operation type identifier.
func (o *FsWriteAtOp) GetOperationTypeId() string {
	return FsWriteAtOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *FsWriteAtOp) ApplyWorldOp(
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
func (o *FsWriteAtOp) ApplyWorldObjectOp(
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
			ftree, err := unixfs_block.NewFSTree(ctx, bcs, unixfs_block.NodeType_NodeType_UNKNOWN)
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
func (o *FsWriteAtOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *FsWriteAtOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = ((*FsWriteAtOp)(nil))
