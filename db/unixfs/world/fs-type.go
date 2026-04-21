package unixfs_world

import (
	"context"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
)

const (
	// FSObjectTypeID is the type identifier for FSObject.
	FSObjectTypeID = "unixfs/fs-object"
	// FSNodeTypeID is the type identifier for the FS node.
	FSNodeTypeID = "unixfs/fs-node"
	// FSHostVolumeTypeID is the type identifier for the host volume FS node.
	FSHostVolumeTypeID = "unixfs/fs-host-volume"
)

// TypeIDToFSType converts a TypeID to a FSType.
// returns an error if the typeID was not recognized.
// if the string is empty, returns UNKNOWN.
func TypeIDToFSType(typeID string) (FSType, error) {
	switch typeID {
	case "":
		return FSType_FSType_UNKNOWN, nil
	case FSNodeTypeID:
		return FSType_FSType_FS_NODE, nil
	case FSObjectTypeID:
		return FSType_FSType_FS_OBJECT, nil
	case FSHostVolumeTypeID:
		return FSType_FSType_FS_HOST_VOLUME, nil
	default:
		return 0, errors.Wrap(ErrInvalidFSType, typeID)
	}
}

// FSTypeToTypeID converts a FSType to a TypeID.
// returns an error if the typeID was not recognized.
func FSTypeToTypeID(fsType FSType) (string, error) {
	switch fsType {
	case FSType_FSType_UNKNOWN:
		return "", nil
	case FSType_FSType_FS_HOST_VOLUME:
		return FSHostVolumeTypeID, nil
	case FSType_FSType_FS_NODE:
		return FSNodeTypeID, nil
	case FSType_FSType_FS_OBJECT:
		return FSObjectTypeID, nil
	default:
		return "", errors.Wrap(ErrInvalidFSType, fsType.String())
	}
}

// GetFSRootWithType creates a filesystem root from type.
// defaults to type FS_NODE.
// returns the block ctor, type ID, and error.
func GetFSRootWithType(f FSType) (block.Ctor, string, error) {
	switch f {
	case FSType_FSType_UNKNOWN:
		fallthrough
	case FSType_FSType_FS_NODE:
		return unixfs_block.NewFSNodeBlock, FSNodeTypeID, nil
	case FSType_FSType_FS_OBJECT:
		return unixfs_block.NewFSObjectBlock, FSObjectTypeID, nil
	case FSType_FSType_FS_HOST_VOLUME:
		return unixfs_block.NewFSHostVolumeBlock, FSHostVolumeTypeID, nil
	}
	return nil, "", errors.Wrap(ErrInvalidFSType, f.String())
}

// NewFSRootWithType constructs a new FSRoot with the given type.
// defaults to type FS_NODE.
// returns the block, type ID, and error.
func NewFSRootWithType(f FSType, rootType unixfs.FSCursorNodeType, ts *timestamp.Timestamp) (block.Block, string, error) {
	rootNt := unixfs_block.FSCursorNodeTypeToNodeType(rootType)
	switch f {
	case FSType_FSType_UNKNOWN:
		fallthrough
	case FSType_FSType_FS_NODE:
		nod := unixfs_block.NewFSNode(rootNt, 0, ts)
		return nod, FSNodeTypeID, nil
	case FSType_FSType_FS_OBJECT:
		obj := unixfs_block.NewFSObject(ts, unixfs_block.NewFSNode(rootNt, 0, ts))
		return obj, FSObjectTypeID, nil
	case FSType_FSType_FS_HOST_VOLUME:
		obj := unixfs_block.NewFSHostVolume("")
		return obj, FSHostVolumeTypeID, nil
	}
	return nil, "", errors.Wrap(ErrInvalidFSType, f.String())
}

// UnmarshalFSRootWithType unmarshals the filesystem root by type.
// returns nil, typeID, nil if the root was empty.
// returns the block, type ID, and error.
func UnmarshalFSRootWithType(ctx context.Context, bcs *block.Cursor, f FSType) (block.Block, string, error) {
	ctor, typeID, err := GetFSRootWithType(f)
	if err != nil {
		return nil, "", err
	}
	blk, err := bcs.Unmarshal(ctx, ctor)
	if err != nil {
		return nil, "", err
	}
	if blk == nil {
		return nil, typeID, nil
	}
	return blk, typeID, nil
}

// Validate checks the FSType.
func (f FSType) Validate(allowUnknown bool) error {
	if f == FSType_FSType_UNKNOWN && allowUnknown {
		return nil
	}

	switch f {
	case FSType_FSType_FS_NODE:
	case FSType_FSType_FS_OBJECT:
	case FSType_FSType_FS_HOST_VOLUME:
	default:
		return errors.Wrap(ErrInvalidFSType, f.String())
	}

	return nil
}
