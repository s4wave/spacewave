package unixfs_world

import (
	"context"
	"time"

	"github.com/aperturerobotics/hydra/block"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/aperturerobotics/hydra/world"
	"github.com/aperturerobotics/timestamp"
	"github.com/golang/protobuf/proto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/pkg/errors"
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
) error {
	fpath := unixfs_block.NewFSPath(path)
	wOp := NewFsTruncateOp("", fsType, fpath, size, ts)
	_, _, err := obj.ApplyObjectOp(wOp, sender)
	return err
}

// FsTruncateOpId is the operation id.
var FsTruncateOpId = "hydra/unixfs/truncate"

// NewFsTruncateOp constructs a new FsTruncateOp block.
// repoRef, worktreeArgs can be empty
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

// NewFsTruncateOpBlock constructs a new FsTruncateOp block.
func NewFsTruncateOpBlock() block.Block {
	return &FsTruncateOp{}
}

// Validate performs cursory checks on the op.
func (o *FsTruncateOp) Validate() error {
	if o.GetTimestamp().GetTimeUnixMs() == 0 {
		return unixfs_errors.ErrEmptyTimestamp
	}
	if err := o.GetFsType().Validate(true); err != nil {
		return err
	}
	if o.GetFileSize() < 0 {
		return errors.Errorf("file size cannot be less than zero: %d", o.GetFileSize())
	}
	return nil
}

// GetOperationTypeId returns the operation type identifier.
func (o *FsTruncateOp) GetOperationTypeId() string {
	return FsTruncateOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *FsTruncateOp) ApplyWorldOp(
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
func (o *FsTruncateOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	// validate
	err = unixfs_block.ValidateTruncate(o.GetPath(), o.GetFileSize())
	if err != nil {
		return false, err
	}

	// TODO: pass timestamp to ApplyWorldObjectOp
	var ts *timestamp.Timestamp

	_, _, err = world.AccessObjectState(ctx, objectHandle, true, func(bcs *block.Cursor) error {
		switch o.GetFsType() {
		case FSType_FSType_FS_NODE:
			ftree, err := unixfs_block.NewFSTree(bcs, unixfs_block.NodeType_NodeType_UNKNOWN)
			if err != nil {
				return err
			}
			// wr := unixfs_block.NewFSWriter(ftree)
			fpath := o.GetPath().GetNodes()
			return unixfs_block.TruncateFile(ctx, ftree, fpath, o.GetFileSize(), ts)
		case FSType_FSType_FS_OBJECT:
			return errors.New("TODO apply truncate to fsobject")
		default:
			return errors.Wrap(ErrInvalidFSType, o.GetFsType().String())
		}
	})
	return false, err
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (o *FsTruncateOp) MarshalBlock() ([]byte, error) {
	return proto.Marshal(o)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (o *FsTruncateOp) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, o)
}

// _ is a type assertion
var _ world.Operation = ((*FsTruncateOp)(nil))
