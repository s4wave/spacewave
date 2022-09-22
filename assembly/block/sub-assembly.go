package assembly_block

import (
	"github.com/aperturerobotics/bldr/assembly"
	"github.com/aperturerobotics/hydra/block"
)

// NewSubAssemblyBlock builds a new SubAssembly block.
func NewSubAssemblyBlock() block.Block {
	return &SubAssembly{}
}

// BuildCursor builds the SubAssembly cursor.
func (r *SubAssembly) BuildCursor(bcs *block.Cursor) assembly.SubAssembly {
	return NewSubAssemblyCursor(r, bcs)
}

// MarshalBlock marshals the block to binary.
func (r *SubAssembly) MarshalBlock() ([]byte, error) {
	return r.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (r *SubAssembly) UnmarshalBlock(data []byte) error {
	return r.UnmarshalVT(data)
}

// ApplySubBlock applies a sub-block change with a field id.
func (r *SubAssembly) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 1:
		v, ok := next.(*assemblySet)
		if !ok {
			// ignore
			return nil
		}
		if v.r == nil {
			r.Assemblies = nil
		} else {
			r.Assemblies = *v.r
		}
		return nil
	case 3:
		v, ok := next.(*directiveBridgeSet)
		if !ok {
			// ignore
			return nil
		}
		if v.r == nil {
			r.DirectiveBridges = nil
		} else {
			r.DirectiveBridges = *v.r
		}
		return nil
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (r *SubAssembly) GetSubBlocks() map[uint32]block.SubBlock {
	v := make(map[uint32]block.SubBlock)
	v[1] = NewAssemblySet(&r.Assemblies, nil)
	v[3] = NewDirectiveBridgeSet(&r.DirectiveBridges, nil)
	return v
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (r *SubAssembly) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 1:
		return NewAssemblySetSubBlockCtor(&r.Assemblies)
	case 3:
		return NewDirectiveBridgeSetSubBlockCtor(&r.DirectiveBridges)
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*SubAssembly)(nil))
	_ block.BlockWithSubBlocks = ((*SubAssembly)(nil))
)
