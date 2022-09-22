package assembly_block

import (
	"github.com/aperturerobotics/bldr/assembly"
	controller_exec "github.com/aperturerobotics/controllerbus/controller/exec"
	"github.com/aperturerobotics/hydra/block"
)

// NewAssemblyBlock builds a new Assembly block.
func NewAssemblyBlock() block.Block {
	return &Assembly{}
}

// UnmarshalAssembly unmarshals a Assembly from a cursor.
// If empty, returns nil, nil
func UnmarshalAssembly(bcs *block.Cursor) (*Assembly, error) {
	if bcs == nil {
		return nil, nil
	}
	blk, err := bcs.Unmarshal(NewAssemblyBlock)
	if err != nil {
		return nil, err
	}
	if blk == nil {
		return nil, nil
	}
	bv, ok := blk.(*Assembly)
	if !ok {
		return nil, block.ErrUnexpectedType
	}
	return bv, nil
}

// BuildCursor builds the Assembly cursor.
func (r *Assembly) BuildCursor(bcs *block.Cursor) assembly.Assembly {
	return NewAssemblyCursor(r, bcs)
}

// MarshalBlock marshals the block to binary.
func (r *Assembly) MarshalBlock() ([]byte, error) {
	return r.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (r *Assembly) UnmarshalBlock(data []byte) error {
	return r.UnmarshalVT(data)
}

// ApplySubBlock applies a sub-block change with a field id.
func (r *Assembly) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 1:
		v, ok := next.(*controller_exec.ExecControllerRequest)
		if !ok {
			return block.ErrUnexpectedType
		}
		r.ControllerExec = v
	case 2:
		v, ok := next.(*subAssemblySet)
		if !ok {
			// ignore
			return nil
		}
		if v.r == nil || v == nil {
			r.SubAssemblies = nil
		} else {
			r.SubAssemblies = *v.r
		}
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (r *Assembly) GetSubBlocks() map[uint32]block.SubBlock {
	v := make(map[uint32]block.SubBlock)
	v[1] = r.GetControllerExec()
	v[2] = NewSubAssemblySet(&r.SubAssemblies, nil)
	return v
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (r *Assembly) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 1:
		return func(create bool) block.SubBlock {
			v := r.GetControllerExec()
			if create && v == nil {
				r.ControllerExec = &controller_exec.ExecControllerRequest{}
				v = r.ControllerExec
			}
			return v
		}
	case 2:
		return NewSubAssemblySetSubBlockCtor(&r.SubAssemblies)
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*Assembly)(nil))
	_ block.BlockWithSubBlocks = ((*Assembly)(nil))
)
