package git_block

import (
	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/golang/protobuf/proto"
)

// NewResolveUndoEntry constructs a new resolve undo entry.
func NewResolveUndoEntry(e *index.ResolveUndoEntry) (*ResolveUndoEntry, error) {
	if e == nil {
		return nil, nil
	}

	st := make(map[uint32]*hash.Hash, len(e.Stages))
	for k, v := range e.Stages {
		var err error
		st[uint32(k)], err = NewHash(v)
		if err != nil {
			return nil, err
		}
	}

	return &ResolveUndoEntry{
		Path:   e.Path,
		Stages: st,
	}, nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (e *ResolveUndoEntry) MarshalBlock() ([]byte, error) {
	return proto.Marshal(e)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (e *ResolveUndoEntry) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, e)
}

// ApplySubBlock applies a sub-block change with a field id.
func (e *ResolveUndoEntry) ApplySubBlock(id uint32, next block.SubBlock) error {
	if id == 2 {
		v, ok := next.(*StagesMap)
		if !ok || v == nil || v.v == nil {
			// ignore
			return nil
		}
		e.Stages = *v.v
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (e *ResolveUndoEntry) GetSubBlocks() map[uint32]block.SubBlock {
	out := make(map[uint32]block.SubBlock)
	out[2] = NewStagesMap(&e.Stages)
	return out
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (e *ResolveUndoEntry) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	if id == 2 {
		return func(create bool) block.SubBlock {
			if e == nil || e.Stages == nil {
				if !create || e == nil {
					return nil
				}
				e.Stages = make(map[uint32]*hash.Hash)
			}
			return NewStagesMap(&e.Stages)
		}
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*ResolveUndoEntry)(nil))
	_ block.BlockWithSubBlocks = ((*ResolveUndoEntry)(nil))
)
