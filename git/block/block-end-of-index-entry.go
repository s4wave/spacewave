package git_block

import (
	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	"github.com/go-git/go-git/v5/plumbing/format/index"
)

// NewEndOfIndexEntry constructs a EndOfIndexEntry from the git block.
func NewEndOfIndexEntry(e *index.EndOfIndexEntry) (*EndOfIndexEntry, error) {
	if e == nil {
		return nil, nil
	}

	h, err := NewHash(e.Hash)
	if err != nil {
		return nil, err
	}

	return &EndOfIndexEntry{
		Offset: e.Offset,
		Hash:   h,
	}, nil
}

// NewEndOfIndexEntryBlock builds a new repo root block.
func NewEndOfIndexEntryBlock() block.Block {
	return &EndOfIndexEntry{}
}

// IsNil returns if the object is nil.
func (r *EndOfIndexEntry) IsNil() bool {
	return r == nil
}

// ToGitEndOfIndexEntry converts to the git EndOfIndexEntry object.
func (r *EndOfIndexEntry) ToGitEndOfIndexEntry() (*index.EndOfIndexEntry, error) {
	if r == nil || (len(r.GetHash().GetHash()) == 0 && r.GetOffset() == 0) {
		return nil, nil
	}
	out := &index.EndOfIndexEntry{Offset: r.GetOffset()}
	var err error
	out.Hash, err = FromHash(r.GetHash())
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Validate performs cursory checks on the repo block.
func (r *EndOfIndexEntry) Validate() error {
	if err := r.GetHash().Validate(); err != nil {
		return err
	}
	return nil
}

// MarshalBlock marshals the block to binary.
func (r *EndOfIndexEntry) MarshalBlock() ([]byte, error) {
	return r.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (r *EndOfIndexEntry) UnmarshalBlock(data []byte) error {
	return r.UnmarshalVT(data)
}

// ApplySubBlock applies a sub-block change with a field id.
func (r *EndOfIndexEntry) ApplySubBlock(id uint32, next block.SubBlock) error {
	if id == 2 {
		v, ok := next.(*hash.Hash)
		if !ok {
			return block.ErrUnexpectedType
		}
		r.Hash = v
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (r *EndOfIndexEntry) GetSubBlocks() map[uint32]block.SubBlock {
	v := make(map[uint32]block.SubBlock)
	v[2] = r.GetHash()
	return v
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (r *EndOfIndexEntry) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	if id == 2 {
		return func(create bool) block.SubBlock {
			v := r.GetHash()
			if create && v == nil {
				v = &hash.Hash{}
				r.Hash = v
			}
			return v
		}
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*EndOfIndexEntry)(nil))
	_ block.BlockWithSubBlocks = ((*EndOfIndexEntry)(nil))
)
