package unixfs_world

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	"github.com/aperturerobotics/timestamp"
	"github.com/pkg/errors"
)

const (
	// FSObjectTypeID is the type identifier for FSObject.
	FSObjectTypeID = "unixfs/fs"
	// FSNodeTypeID is the type identifier for the FS node.
	FSNodeTypeID = "unixfs/fs-node"
)

// TypeIDToFSType converts a TypeID to a FSType.
// returns an error if the typeID was not recognized.
// if the string is empty, returns UNKNOWN.
func TypeIDToFSType(typeID string) (FSType, error) {
	if typeID == "" {
		return FSType_FSType_UNKNOWN, nil
	}

	switch typeID {
	case FSNodeTypeID:
		return FSType_FSType_FS_NODE, nil
	case FSObjectTypeID:
		return FSType_FSType_FS_OBJECT, nil
	default:
		return 0, errors.Wrap(ErrInvalidFSType, typeID)
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
	case FSType_FSType_FS_OBJECT:
		obj := unixfs_block.NewFSObject(ts, unixfs_block.NewFSNode(rootNt, 0, ts))
		return obj, FSObjectTypeID, nil
	case FSType_FSType_FS_NODE:
		nod := unixfs_block.NewFSNode(rootNt, 0, ts)
		return nod, FSNodeTypeID, nil
	}
	return nil, "", errors.Wrap(ErrInvalidFSType, f.String())
}

// UnmarshalFSRootWithType unmarshals the filesystem root by type.
// returns nil, typeID, nil if the root was empty.
// returns the block, type ID, and error.
func UnmarshalFSRootWithType(bcs *block.Cursor, f FSType) (block.Block, string, error) {
	ctor, typeID, err := GetFSRootWithType(f)
	blk, err := bcs.Unmarshal(ctor)
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
	default:
		return errors.Wrap(ErrInvalidFSType, f.String())
	}

	return nil
}
