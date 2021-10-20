package git_block

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/golang/protobuf/proto"
)

// NewResolveUndo constructs a new resolve undo block from the git block.
func NewResolveUndo(u *index.ResolveUndo) (*ResolveUndo, error) {
	if u == nil {
		return nil, nil
	}
	entries := make([]*ResolveUndoEntry, len(u.Entries))
	for i := range u.Entries {
		var err error
		entries[i], err = NewResolveUndoEntry(&u.Entries[i])
		if err != nil {
			return nil, err
		}
	}
	return &ResolveUndo{Entries: entries}, nil
}

// NewResolveUndoBlock builds a new resolve undo block.
func NewResolveUndoBlock() block.Block {
	return &ResolveUndo{}
}

// ToGitResolveUndo converts to a git resolve undo block.
func (i *ResolveUndo) ToGitResolveUndo() (*index.ResolveUndo, error) {
	if i == nil || len(i.GetEntries()) == 0 {
		return nil, nil
	}

	ents := i.GetEntries()
	out := make([]index.ResolveUndoEntry, len(ents))
	for i, e := range ents {
		stg := make(map[index.Stage]plumbing.Hash, len(e.GetStages()))
		for k, v := range e.GetStages() {
			var err error
			stg[index.Stage(k)], err = FromHash(v)
			if err != nil {
				return nil, err
			}
		}
		out[i] = index.ResolveUndoEntry{
			Path:   e.GetPath(),
			Stages: stg,
		}
	}
	return &index.ResolveUndo{
		Entries: out,
	}, nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (i *ResolveUndo) MarshalBlock() ([]byte, error) {
	return proto.Marshal(i)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (i *ResolveUndo) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, i)
}

// ApplySubBlock applies a sub-block change with a field id.
func (i *ResolveUndo) ApplySubBlock(id uint32, next block.SubBlock) error {
	// no-op
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (i *ResolveUndo) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	m[1] = NewResolveUndoEntrySet(&i.Entries, nil)
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (i *ResolveUndo) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 1:
		return func(create bool) block.SubBlock {
			return NewResolveUndoEntrySet(&i.Entries, nil)
		}
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*ResolveUndo)(nil))
	_ block.BlockWithSubBlocks = ((*ResolveUndo)(nil))
)
