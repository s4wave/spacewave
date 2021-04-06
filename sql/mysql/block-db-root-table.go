package mysql

import (
	"github.com/aperturerobotics/hydra/block"
)

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (r *DatabaseRootTable) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case 2:
		r.Ref = ptr
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (r *DatabaseRootTable) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	if r == nil {
		return nil, nil
	}
	m := make(map[uint32]*block.BlockRef)
	m[2] = r.Ref
	return m, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (r *DatabaseRootTable) GetBlockRefCtor(id uint32) block.Ctor {
	switch id {
	case 2:
		return NewTableRootBlock
	}
	return nil
}

// _ is a type assertion
var (
	_ block.SubBlock      = (*DatabaseRootTable)(nil)
	_ block.BlockWithRefs = (*DatabaseRootTable)(nil)
)
