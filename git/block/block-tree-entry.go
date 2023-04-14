package git_block

import (
	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	"github.com/go-git/go-git/v5/plumbing/format/index"
)

// NewTreeEntry constructs a new tree entry from a git tree entry.
func NewTreeEntry(t *index.TreeEntry) (*TreeEntry, error) {
	if t == nil {
		return nil, nil
	}
	out := &TreeEntry{
		Path:    t.Path,
		Entries: int32(t.Entries),
		Trees:   int32(t.Trees),
	}
	var err error
	out.Hash, err = NewHash(t.Hash)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// NewTreeEntryBlock builds a new tree entry block.
func NewTreeEntryBlock() block.Block {
	return &TreeEntry{}
}

// IsNil returns if the object is nil.
func (i *TreeEntry) IsNil() bool {
	return i == nil
}

// Validate performs cursory validation of the tree entry.
func (i *TreeEntry) Validate() error {
	if err := i.GetHash().Validate(); err != nil {
		return err
	}
	return nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (i *TreeEntry) MarshalBlock() ([]byte, error) {
	return i.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (i *TreeEntry) UnmarshalBlock(data []byte) error {
	return i.UnmarshalVT(data)
}

// ApplySubBlock applies a sub-block change with a field id.
func (i *TreeEntry) ApplySubBlock(id uint32, next block.SubBlock) error {
	if id == 4 {
		v, ok := next.(*hash.Hash)
		if !ok {
			return block.ErrUnexpectedType
		}
		i.Hash = v
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (i *TreeEntry) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	m[4] = i.GetHash()
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (i *TreeEntry) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	if id == 4 {
		return func(create bool) block.SubBlock {
			v := i.GetHash()
			if create && v == nil {
				v = &hash.Hash{}
				i.Hash = v
			}
			return v
		}
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*TreeEntry)(nil))
	_ block.BlockWithSubBlocks = ((*TreeEntry)(nil))
)
