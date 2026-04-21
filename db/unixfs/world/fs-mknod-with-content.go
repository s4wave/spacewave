package unixfs_world

import (
	"context"
	"io"
	"io/fs"
	"time"

	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// FsMknodWithContent creates a file with content atomically.
// Phase 1: pre-builds the blob in an isolated object.
// Phase 2: creates the file entry and writes the blob in a single commit.
func FsMknodWithContent(
	ctx context.Context,
	obj world.ObjectState,
	sender peer.ID,
	fsType FSType,
	path []string,
	nodeType unixfs.FSCursorNodeType,
	dataLen int64,
	rdr io.Reader,
	permissions fs.FileMode,
	ts time.Time,
) (rev uint64, sysErr bool, err error) {
	// Phase 1: build the blob in an isolated object.
	fpath := unixfs_block.NewFSPath(path, false)
	blbObjRef, err := world.AccessObject(
		ctx,
		obj.AccessWorldState,
		nil,
		func(bcs *block.Cursor) error {
			bcs.SetRefAtCursor(nil, true)
			_, err := blob.BuildBlob(ctx, dataLen, rdr, bcs, nil)
			return err
		},
	)
	if err != nil {
		return 0, true, err
	}

	// Phase 2: create file entry + write blob content in a single commit.
	tType := unixfs_block.FSCursorNodeTypeToNodeType(nodeType)
	wOp := NewFsMknodWithContentOp("", fsType, fpath, tType, permissions, ts, blbObjRef.GetRootRef())
	return obj.ApplyObjectOp(ctx, wOp, sender)
}

// FsMknodWithContentOpId is the operation id.
var FsMknodWithContentOpId = "hydra/unixfs/mknod-with-content"

// NewFsMknodWithContentOp constructs a new FsMknodWithContentOp block.
func NewFsMknodWithContentOp(
	objKey string,
	fsType FSType,
	path *unixfs_block.FSPath,
	nodeType unixfs_block.NodeType,
	permissions fs.FileMode,
	ts time.Time,
	blbRef *block.BlockRef,
) *FsMknodWithContentOp {
	return &FsMknodWithContentOp{
		ObjectKey:   objKey,
		FsType:      fsType,
		Path:        path,
		NodeType:    nodeType,
		Timestamp:   unixfs_block.ToTimestamp(ts, true),
		Permissions: uint32(permissions.Perm()),
		BlobRef:     blbRef,
	}
}

// NewFsMknodWithContentOpBlock constructs a new FsMknodWithContentOp block.
func NewFsMknodWithContentOpBlock() block.Block {
	return &FsMknodWithContentOp{}
}

// Validate performs cursory checks on the op.
func (o *FsMknodWithContentOp) Validate() error {
	if o.GetPath() == nil || len(o.GetPath().GetNodes()) == 0 {
		return errors.New("path is required")
	}
	if err := o.GetTimestamp().Validate(false); err != nil {
		return err
	}
	if err := o.GetFsType().Validate(true); err != nil {
		return err
	}
	if err := o.GetBlobRef().Validate(false); err != nil {
		return err
	}
	return nil
}

// GetOperationTypeId returns the operation type identifier.
func (o *FsMknodWithContentOp) GetOperationTypeId() string {
	return FsMknodWithContentOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *FsMknodWithContentOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	obj, err := world.MustGetObject(ctx, worldHandle, o.GetObjectKey())
	if err != nil {
		return false, err
	}
	return o.ApplyWorldObjectOp(ctx, le, obj, sender)
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *FsMknodWithContentOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	if err := o.Validate(); err != nil {
		return false, err
	}

	_, _, err = world.AccessObjectState(ctx, objectHandle, true, func(bcs *block.Cursor) error {
		switch o.GetFsType() {
		case FSType_FSType_FS_NODE:
			ftree, err := unixfs_block.NewFSTree(ctx, bcs, unixfs_block.NodeType_NodeType_UNKNOWN)
			if err != nil {
				return err
			}

			return unixfs_block.MknodWithContent(
				ctx,
				ftree,
				o.GetPath().GetNodes(),
				o.GetNodeType(),
				fs.FileMode(o.GetPermissions()),
				o.GetTimestamp(),
				o.GetBlobRef(),
			)
		case FSType_FSType_FS_OBJECT:
			return errors.New("TODO apply mknod-with-content to fsobject")
		case FSType_FSType_FS_HOST_VOLUME:
			return unixfs_block.ErrCannotModifyHostVolume
		default:
			return errors.Wrap(ErrInvalidFSType, o.GetFsType().String())
		}
	})
	return false, err
}

// MarshalBlock marshals the block to binary.
func (o *FsMknodWithContentOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *FsMknodWithContentOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = ((*FsMknodWithContentOp)(nil))
