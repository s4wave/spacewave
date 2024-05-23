package mysql

import (
	"github.com/aperturerobotics/hydra/block"
	kvtx_block "github.com/aperturerobotics/hydra/kvtx/block"
)

// IsNil returns if the object is nil.
func (r *TablePartitionRoot) IsNil() bool {
	return r == nil
}

// Validate performs cursory validation of the table partition root.
func (r *TablePartitionRoot) Validate() error {
	if err := r.GetRowKeyValue().Validate(); err != nil {
		return err
	}
	return nil
}

// ApplySubBlock applies a sub-block change with a field id.
func (r *TablePartitionRoot) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 1:
		return block.ApplySubBlock(&r.RowKeyValue, next)
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (r *TablePartitionRoot) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	if rkv := r.GetRowKeyValue(); rkv != nil {
		m[1] = rkv
	}
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (r *TablePartitionRoot) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 1:
		return kvtx_block.NewKeyValueStoreSubBlockCtor(&r.RowKeyValue)
	}
	return nil
}

// _ is a type assertion
var (
	_ block.SubBlock           = ((*TablePartitionRoot)(nil))
	_ block.BlockWithSubBlocks = ((*TablePartitionRoot)(nil))
)
