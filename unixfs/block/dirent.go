package unixfs_block

import (
	"github.com/aperturerobotics/hydra/block"
)

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (d *Dirent) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case 2:
		d.NodeRef = ptr
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (d *Dirent) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	m := make(map[uint32]*block.BlockRef)
	m[2] = d.GetNodeRef()
	return m, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (d *Dirent) GetBlockRefCtor(id uint32) block.Ctor {
	return NewNodeBlock
}

// _ is a type assertion
var (
	_ block.BlockWithRefs = ((*Dirent)(nil))
)
