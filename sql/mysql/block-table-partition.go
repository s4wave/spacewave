package mysql

import (
	"github.com/aperturerobotics/hydra/block"
	iavl "github.com/aperturerobotics/hydra/kvtx/block/iavl"
	"github.com/pkg/errors"
)

// Validate performs cursory validation of the table partition root.
func (r *TablePartitionRoot) Validate() error {
	if err := r.GetTreeRef().Validate(); err != nil {
		return err
	}
	if v := r.GetPartitionImpl(); v != PartitionImpl_PartitionImpl_IAVL {
		return errors.Errorf("unknown partition impl: %s", v.String())
	}
	return nil
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (r *TablePartitionRoot) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case 1:
		r.TreeRef = ptr
		return nil
	}
	return errors.Errorf("unexpected reference id: %d", id)
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (r *TablePartitionRoot) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	return map[uint32]*block.BlockRef{
		1: r.GetTreeRef(),
	}, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (r *TablePartitionRoot) GetBlockRefCtor(id uint32) block.Ctor {
	switch id {
	case 1:
		return iavl.NewNodeBlock
	}
	return nil
}

// _ is a type assertion
var (
	_ block.SubBlock      = ((*TablePartitionRoot)(nil))
	_ block.BlockWithRefs = ((*TablePartitionRoot)(nil))
)
