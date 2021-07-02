package forge_pass

import (
	forge_target "github.com/aperturerobotics/forge/target"
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/block"
	"github.com/golang/protobuf/proto"
)

// NewPassBlock constructs a new Pass block.
func NewPassBlock() block.Block {
	return &Pass{}
}

// UnmarshalPass unmarshals a pass block from the cursor.
func UnmarshalPass(bcs *block.Cursor) (*Pass, error) {
	vi, err := bcs.Unmarshal(NewPassBlock)
	if err != nil {
		return nil, err
	}
	if vi == nil {
		return nil, nil
	}
	b, ok := vi.(*Pass)
	if !ok {
		return nil, block.ErrUnexpectedType
	}
	return b, nil
}

// IsComplete checks if the execution is in the COMPLETE state.
func (e *Pass) IsComplete() bool {
	return e.GetPassState() == State_PassState_COMPLETE
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (e *Pass) MarshalBlock() ([]byte, error) {
	return proto.Marshal(e)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (e *Pass) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, e)
}

// ApplySubBlock applies a sub-block change with a field id.
func (e *Pass) ApplySubBlock(id uint32, next block.SubBlock) error {
	// no-op
	switch id {
	case 3:
		v, ok := next.(*forge_target.ValueSet)
		if !ok {
			return block.ErrUnexpectedType
		}
		e.ValueSet = v
	case 5:
		v, ok := next.(*forge_value.Result)
		if !ok {
			return block.ErrUnexpectedType
		}
		e.Result = v
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (e *Pass) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	m[3] = e.GetValueSet()
	m[5] = e.GetResult()
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (e *Pass) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 3:
		return forge_target.NewValueSetSubBlockCtor(&e.ValueSet)
	case 5:
		return forge_value.NewResultSubBlockCtor(&e.Result)
	}
	return nil
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (e *Pass) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case 4:
		e.TargetRef = ptr
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (e *Pass) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	m := make(map[uint32]*block.BlockRef)
	m[4] = e.GetTargetRef()
	return m, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (e *Pass) GetBlockRefCtor(id uint32) block.Ctor {
	switch id {
	case 4:
		return forge_target.NewTargetBlock
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*Pass)(nil))
	_ block.BlockWithSubBlocks = ((*Pass)(nil))
	_ block.BlockWithRefs      = ((*Pass)(nil))
)
