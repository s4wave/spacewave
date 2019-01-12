package cid

import (
	"github.com/aperturerobotics/hydra/hash"
	"github.com/golang/protobuf/proto"
	b58 "github.com/mr-tron/base58/base58"
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

// GetEmpty returns if the ref is empty.
func (b *BlockRef) GetEmpty() bool {
	return len(b.GetHash().GetHash()) == 0
}

// EqualsRef checks if two refs are equal.
func (b *BlockRef) EqualsRef(oref *BlockRef) bool {
	return proto.Equal(oref, b)
}

// MarshalKey marshals the block ref for use as a key.
// The format should be reproducible and identical between versions.
func (b *BlockRef) MarshalKey() ([]byte, error) {
	return proto.Marshal(b)
}

// MarshalString marshals the reference to a string form.
func (b *BlockRef) MarshalString() string {
	if b == nil {
		return ""
	}
	dat, err := proto.Marshal(b)
	if err != nil {
		return ""
	}
	return b58.Encode(dat)
}

// UnmarshalString unmarshals a string block ref.
func UnmarshalString(ref string) (*BlockRef, error) {
	if ref == "" {
		return nil, nil
	}

	dat, err := b58.Decode(ref)
	if err != nil {
		return nil, err
	}
	r := &BlockRef{}
	if err := proto.Unmarshal(dat, r); err != nil {
		return nil, err
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return r, nil
}
