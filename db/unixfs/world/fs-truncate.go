package unixfs_world

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// FsTruncate shrinks or extends a file to the specified size.
func FsTruncate(
	ctx context.Context,
	obj world.ObjectState,
	sender peer.ID,
	fsType FSType,
	path []string,
	size int64,
	ts time.Time,
) (rev uint64, sysErr bool, err error) {
	fpath := unixfs_block.NewFSPath(path, false)
	wOp := NewFsTruncateOp("", fsType, fpath, size, ts)
	return obj.ApplyObjectOp(ctx, wOp, sender)
}

// NewFsTruncateOp constructs a new FsTruncateOp block.
// repoRef and worktreeArgs can be empty.
func NewFsTruncateOp(
	objKey string,
	fsType FSType,
	path *unixfs_block.FSPath,
	size int64,
	ts time.Time,
) *FsTruncateOp {
	return &FsTruncateOp{
		ObjectKey: objKey,
		FsType:    fsType,
		Path:      path,
		FileSize:  size,
		Timestamp: unixfs_block.ToTimestamp(ts, true),
	}
}

// Validate performs cursory checks on the op.
func (o *FsTruncateOp) Validate() error {
	if err := o.GetTimestamp().Validate(false); err != nil {
		return err
	}
	if err := o.GetFsType().Validate(true); err != nil {
		return err
	}
	if o.GetFileSize() < 0 {
		return errors.Errorf("file size cannot be less than zero: %d", o.GetFileSize())
	}
	return nil
}

// ApplyWorldOp applies the operation as a world operation.
func (o *FsTruncateOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// get the fs object
	obj, err := world.MustGetObject(ctx, worldHandle, o.GetObjectKey())
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return false, err
	}

	return o.ApplyWorldObjectOp(ctx, le, obj, sender)
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *FsTruncateOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	// validate
	err = unixfs_block.ValidateTruncate(o.GetPath(), o.GetFileSize())
	if err != nil {
		return false, world.NewOpRefusedError(err)
	}

	_, _, err = world.AccessObjectState(ctx, objectHandle, true, func(bcs *block.Cursor) error {
		switch o.GetFsType() {
		case FSType_FSType_FS_NODE:
			ftree, err := unixfs_block.NewFSTree(ctx, bcs, unixfs_block.NodeType_NodeType_UNKNOWN)
			if err != nil {
				return err
			}
			// wr := unixfs_block.NewFSWriter(ftree)
			fpath := o.GetPath().GetNodes()
			return unixfs_block.TruncateFile(ctx, ftree, fpath, o.GetFileSize(), o.GetTimestamp())
		case FSType_FSType_FS_OBJECT:
			return world.NewOpRefusedError(ErrFSObjectUnsupported)
		case FSType_FSType_FS_HOST_VOLUME:
			return world.NewOpRefusedError(unixfs_block.ErrCannotModifyHostVolume)
		default:
			return world.NewOpRefusedError(errors.Wrap(ErrInvalidFSType, o.GetFsType().String()))
		}
	})
	return false, fsRefusal(err)
}

// _ is a type assertion
var _ world.Operation = (*FsTruncateOp)(nil)
