package file

import (
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/byteslice"
)

// IsNil checks if the object is nil.
func (r *Range) IsNil() bool {
	return r == nil
}

// LessThanRange compares to ranges, returning true if r < other.
func (r *Range) LessThanRange(other *Range) bool {
	// compare start, then nonce, ascending order
	rs := r.GetStart()
	os := other.GetStart()
	if rs == os {
		return r.GetNonce() < other.GetNonce()
	}
	return rs < os
}

// FollowBlob follows the blob reference.
func (r *Range) FollowBlob(bcs *block.Cursor) *block.Cursor {
	return bcs.FollowRef(4, r.GetRef())
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
