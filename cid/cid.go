package cid

import (
	"github.com/aperturerobotics/hydra/hash"
	"github.com/golang/protobuf/proto"
)

// NewBlockRef constructs a new block reference.
func NewBlockRef(hash *hash.Hash) *BlockRef {
	return &BlockRef{Hash: hash}
}

// Validate validates the block ref.
func (b *BlockRef) Validate() error {
	if err := b.GetHash().Validate(); err != nil {
		return err
	}
	return nil
}

// MarshalKey marshals the block ref.
// The format should be reproducible and identical between versions..
func (b *BlockRef) MarshalKey() ([]byte, error) {
	return proto.Marshal(b)
}
