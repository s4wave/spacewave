package git_block

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/go-git/go-git/v5/plumbing"
)

// NewShallowRefsStore constructs a shallow refs store from a list of hashes.
func NewShallowRefsStore(hashes []plumbing.Hash) (*ShallowRefsStore, error) {
	hashSet, err := NewHashSet(hashes)
	if err != nil {
		return nil, err
	}
	return &ShallowRefsStore{ShallowRefs: hashSet}, nil
}

// NewShallowRefsStoreBlock builds a new repo references block.
func NewShallowRefsStoreBlock() block.Block {
	return &ShallowRefsStore{}
}

// MarshalBlock marshals the block to binary.
func (r *ShallowRefsStore) MarshalBlock() ([]byte, error) {
	return r.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (r *ShallowRefsStore) UnmarshalBlock(data []byte) error {
	return r.UnmarshalVT(data)
}

// _ is a type assertion
var _ block.Block = (*ShallowRefsStore)(nil)
