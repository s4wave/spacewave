package git_block

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

// NewTree builds a new tree block from a git tree.
func NewTree(t *index.Tree) (*Tree, error) {
	if t == nil {
		return nil, nil
	}
	entries := make([]*TreeEntry, len(t.Entries))
	for i := range t.Entries {
		var err error
		entries[i], err = NewTreeEntry(&t.Entries[i])
		if err != nil {
			return nil, err
		}
	}
	return &Tree{
		Entries: entries,
	}, nil
}

// NewTreeBlock builds a new tree block.
func NewTreeBlock() block.Block {
	return &Tree{}
}

// Validate performs cursory validation of the tree.
func (i *Tree) Validate() error {
	for idx, ent := range i.GetEntries() {
		if err := ent.Validate(); err != nil {
			return errors.Wrapf(err, "entries[%d]", idx)
		}
	}
	return nil
}

// ToGitTree converts to a git tree.
func (i *Tree) ToGitTree() (*index.Tree, error) {
	if i == nil || len(i.GetEntries()) == 0 {
		return nil, nil
	}

	ents := i.GetEntries()
	out := make([]index.TreeEntry, len(ents))
	for i, e := range ents {
		h, err := FromHash(e.GetHash())
		if err != nil {
			return nil, err
		}
		out[i] = index.TreeEntry{
			Path:    e.GetPath(),
			Entries: int(e.GetEntries()),
			Trees:   int(e.GetTrees()),
			Hash:    h,
		}
	}
	return &index.Tree{
		Entries: out,
	}, nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (i *Tree) MarshalBlock() ([]byte, error) {
	return proto.Marshal(i)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (i *Tree) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, i)
}

// ApplySubBlock applies a sub-block change with a field id.
func (i *Tree) ApplySubBlock(id uint32, next block.SubBlock) error {
	// no-op
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (i *Tree) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	m[1] = NewTreeEntrySet(&i.Entries, nil)
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (i *Tree) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 1:
		return func(create bool) block.SubBlock {
			return NewTreeEntrySet(&i.Entries, nil)
		}
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*Tree)(nil))
	_ block.BlockWithSubBlocks = ((*Tree)(nil))
)
