package unixfs_block

import (
	"github.com/aperturerobotics/hydra/block"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
)

// NewFSObject constructs a new FSObject with defaults.
func NewFSObject(ts *timestamp.Timestamp, rootNode *FSNode) *FSObject {
	// set placeholder if nil
	if rootNode == nil {
		rootNode = NewFSNode(0, 0, ts)
	}
	return &FSObject{
		FsNode: rootNode,
	}
}

// NewFSObjectBlock constructs a new FSObject block.Block.
func NewFSObjectBlock() block.Block {
	return &FSObject{}
}

// FollowFSNode attempts to build a fstree from the state.
func (o *FSObject) FollowFSNode(bcs *block.Cursor) (*FSNode, *block.Cursor, error) {
	return o.GetFsNode(), bcs.FollowSubBlock(1), nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (o *FSObject) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (o *FSObject) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// ApplySubBlock applies a sub-block change with a field id.
func (o *FSObject) ApplySubBlock(id uint32, next block.SubBlock) error {
	var ok bool
	switch id {
	case 1:
		o.FsNode, ok = next.(*FSNode)
		if !ok {
			return block.ErrUnexpectedType
		}
	case 3:
		// o.LastChange, ok = next.(*FSChange)
		o.LastChange, ok = next.(*FSChange)
		if !ok {
			return block.ErrUnexpectedType
		}
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (o *FSObject) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	m[1] = o.GetFsNode()
	m[3] = o.GetLastChange()
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (o *FSObject) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 1:
		return func(create bool) block.SubBlock {
			v := o.FsNode
			if v == nil && create {
				v = &FSNode{}
				o.FsNode = v
			}
			return v
		}
	case 3:
		return func(create bool) block.SubBlock {
			v := o.LastChange
			if v == nil && create {
				v = &FSChange{}
				o.LastChange = v
			}
			return v
		}
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*FSObject)(nil))
	_ block.BlockWithSubBlocks = ((*FSObject)(nil))
)
