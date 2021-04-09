package file

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/byteslice"
)

// LessThanRange compares to ranges, returning true if r < other.
func (r *Range) LessThanRange(other *Range) bool {
	// first check if the start is before
	rs := r.GetStart()
	os := other.GetStart()
	if rs == os {
		return r.GetNonce() < other.GetNonce()
	}
	return rs < os
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (r *Range) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case 4:
		r.Ref = ptr
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (r *Range) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	m := make(map[uint32]*block.BlockRef)
	m[4] = r.GetRef()
	return m, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (r *Range) GetBlockRefCtor(id uint32) block.Ctor {
	switch id {
	case 4:
		return byteslice.NewByteSliceBlock
	}
	return nil
}

// _ is a type assertion
var (
	_ block.BlockWithRefs = ((*Range)(nil))
)
