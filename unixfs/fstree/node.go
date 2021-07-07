package fstree

import (
	"errors"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/file"
	"github.com/golang/protobuf/proto"
)

// NewNodeBlock constructs a Node as a Block.
func NewNodeBlock() block.Block {
	return &Node{}
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (n *Node) MarshalBlock() ([]byte, error) {
	return proto.Marshal(n)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (n *Node) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, n)
}

// ApplySubBlock applies a sub-block change with a field id.
func (n *Node) ApplySubBlock(id uint32, next block.SubBlock) error {
	var ok bool
	switch id {
	case 4:
		n.File, ok = next.(*file.File)
		if !ok {
			return errors.New("sub-block must be a file object")
		}
	case 5:
		var dslice *DirentSlice
		dslice, ok = next.(*DirentSlice)
		if !ok {
			return errors.New("sub-block must be a dirent slice")
		}
		if dslice == nil || dslice.dirents == nil {
			n.DirectoryEntry = nil
		} else {
			n.DirectoryEntry = *dslice.dirents
		}
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (n *Node) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	m[4] = n.GetFile()
	m[5] = NewDirentSlice(&n.DirectoryEntry, nil)
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (n *Node) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 4:
		return func(create bool) block.SubBlock {
			if n.File == nil && create {
				n.File = &file.File{}
			}
			return n.File
		}
	case 5:
		return func(create bool) block.SubBlock {
			return NewDirentSlice(&n.DirectoryEntry, nil)
		}
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*Node)(nil))
	_ block.BlockWithSubBlocks = ((*Node)(nil))
)
