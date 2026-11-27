package bucket_mock

import (
	"github.com/s4wave/spacewave/db/block"
)

// ApplyBlockRef applies a ref change with a field id.
func (r *Root) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case 1:
		r.ExamplePtr.RootRef = ptr
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: the returned map does not include pending references (in a cursor).
func (r *Root) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	return map[uint32]*block.BlockRef{
		1: r.GetExamplePtr().GetRootRef(),
	}, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID.
func (r *Root) GetBlockRefCtor(id uint32) block.Ctor {
	switch id {
	case 1:
		return func() block.Block { return &Root{} }
	}
	return nil
}

// _ is a type assertion
var (
	_ block.BlockWithRefs = (*Root)(nil)
)
