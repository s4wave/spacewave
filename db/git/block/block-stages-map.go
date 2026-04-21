package git_block

import (
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

// StagesMap is a sub-block representing a map from Stage to hash.
type StagesMap struct {
	// v is the pointer to the value
	v *map[uint32]*hash.Hash
}

// NewStagesMap constructs a new StagesMap sub-block.
func NewStagesMap(v *map[uint32]*hash.Hash) *StagesMap {
	return &StagesMap{v: v}
}

// IsNil returns if the object is nil.
func (m *StagesMap) IsNil() bool {
	return m == nil
}

// ApplySubBlock applies a sub-block change with a field id.
func (m *StagesMap) ApplySubBlock(id uint32, next block.SubBlock) error {
	if m == nil || m.v == nil {
		return nil
	}

	// expect sub-block to be a hash
	v, ok := next.(*hash.Hash)
	if !ok {
		return block.ErrUnexpectedType
	}
	sm := *m.v
	if sm == nil {
		sm = make(map[uint32]*hash.Hash)
		*m.v = sm
	}
	sm[id] = v
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (m *StagesMap) GetSubBlocks() map[uint32]block.SubBlock {
	if m == nil || m.v == nil || (*m.v) == nil {
		return nil
	}
	out := make(map[uint32]block.SubBlock)
	for id, hash := range *m.v {
		out[id] = hash
	}
	return out
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (m *StagesMap) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	if m == nil || m.v == nil {
		return nil
	}
	return func(create bool) block.SubBlock {
		ma := *m.v
		if ma == nil {
			if !create {
				return nil
			}
			ma = make(map[uint32]*hash.Hash)
			*m.v = ma
		}
		v, exists := ma[id]
		if create && !exists {
			v = &hash.Hash{}
			ma[id] = v
		}
		return v
	}
}

// _ is a type assertion
var (
	_ block.SubBlock           = ((*StagesMap)(nil))
	_ block.BlockWithSubBlocks = ((*StagesMap)(nil))
)
