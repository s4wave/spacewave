package mysql

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/golang/protobuf/proto"
)

// NewTablePartitionRowBlock constructs a new db root block.
func NewTablePartitionRowBlock() block.Block {
	return &TablePartitionRow{}
}

// MarshalBlock marshals the block to binary.
func (r *TablePartitionRow) MarshalBlock() ([]byte, error) {
	return proto.Marshal(r)
}

// UnmarshalBlock unmarshals the block to the object.
func (r *TablePartitionRow) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, r)
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (r *TablePartitionRow) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case 2:
		r.TableRowRef = ptr
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (r *TablePartitionRow) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	m := make(map[uint32]*block.BlockRef)
	m[2] = r.GetTableRowRef()
	return m, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (r *TablePartitionRow) GetBlockRefCtor(id uint32) block.Ctor {
	switch id {
	case 2:
		return NewTableRowBlock
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block         = (*TablePartitionRow)(nil)
	_ block.SubBlock      = (*TablePartitionRow)(nil)
	_ block.BlockWithRefs = (*TablePartitionRow)(nil)
)
