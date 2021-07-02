package forge_execution

import (
	"github.com/aperturerobotics/bifrost/peer"
	forge_target "github.com/aperturerobotics/forge/target"
	"github.com/aperturerobotics/hydra/block"
	"github.com/golang/protobuf/proto"
)

// NewSpec constructs a execution specification with parameters.
//
// peerID can be empty.
func NewSpec(
	peerID peer.ID,
	targetRef *block.BlockRef,
	valueSet *forge_target.ValueSet,
) *Spec {
	return &Spec{
		PeerId:    peerID.Pretty(),
		ValueSet:  valueSet,
		TargetRef: targetRef,
	}
}

// AssignTarget assigns a target to the spec with a block cursor.
// if tgt is nil clears the field
func (s *Spec) AssignTarget(bcs *block.Cursor, tgt *forge_target.Target) {
	s.TargetRef = nil
	bcs.ClearRef(3)
	if tgt != nil {
		tbcs := bcs.FollowRef(3, nil)
		tbcs.SetBlock(tgt, true)
	}
}

// MarshalBlock marshals the block to binary.
func (s *Spec) MarshalBlock() ([]byte, error) {
	return proto.Marshal(s)
}

// UnmarshalBlock unmarshals the block to the object.
func (s *Spec) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, s)
}

// ApplyBlockRef applies a ref change with a field id.
func (s *Spec) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case 3:
		s.TargetRef = ptr
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
func (s *Spec) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	m := make(map[uint32]*block.BlockRef)
	m[3] = s.GetTargetRef()
	return m, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
func (s *Spec) GetBlockRefCtor(id uint32) block.Ctor {
	switch id {
	case 3:
		return forge_target.NewTargetBlock
	}
	return nil
}

// ApplySubBlock applies a sub-block change with a field id.
func (s *Spec) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 2:
		v, ok := next.(*forge_target.ValueSet)
		if !ok {
			return block.ErrUnexpectedType
		}
		s.ValueSet = v
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (s *Spec) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	m[2] = s.GetValueSet()
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (s *Spec) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 2:
		return forge_target.NewValueSetSubBlockCtor(&s.ValueSet)
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*Spec)(nil))
	_ block.BlockWithRefs      = ((*Spec)(nil))
	_ block.BlockWithSubBlocks = ((*Spec)(nil))
)
