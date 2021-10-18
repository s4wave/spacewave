package assembly_block

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/refslice"
)

// NewAssemblyRefSlice returns a slice of Assembly wrapped as a SubBlock.
func NewAssemblyRefSlice(r *[]*block.BlockRef) *refslice.BlockRefSlice {
	return refslice.NewBlockRefSlice(r, nil, func(idx int) block.Ctor {
		return NewAssemblyBlock
	})
}

// NewAssemblyRefSliceSubBlockCtor returns a sub-block ctor for a AssemblyRefSlice.
func NewAssemblyRefSliceSubBlockCtor(r *[]*block.BlockRef) block.SubBlockCtor {
	return func(create bool) block.SubBlock {
		return NewAssemblyRefSlice(r)
	}
}
