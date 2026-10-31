package block_mock

import (
	"github.com/s4wave/spacewave/db/block"
)

// NewPairBlock constructs a Pair as a Block.
func NewPairBlock() block.Block {
	return &Pair{}
}

// IsNil returns if the object is nil.
func (p *Pair) IsNil() bool {
	return p == nil
}

// ApplyBlockRef applies a ref change with a field id.
func (p *Pair) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case 1:
		p.Left = ptr
	case 2:
		p.Right = ptr
	}
	return nil
}

// GetBlockRefs returns all filled block references.
func (p *Pair) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	return map[uint32]*block.BlockRef{
		1: p.GetLeft(),
		2: p.GetRight(),
	}, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
func (p *Pair) GetBlockRefCtor(id uint32) block.Ctor {
	switch id {
	case 1, 2:
		return NewExampleBlock
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block         = (*Pair)(nil)
	_ block.BlockWithRefs = (*Pair)(nil)
)
