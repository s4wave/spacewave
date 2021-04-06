package block

import (
	"bytes"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/golang/protobuf/proto"
	b58 "github.com/mr-tron/base58/base58"
)

// defaultHashType is the fallback default hash type
const defaultHashType = hash.HashType_HashType_SHA256

// NewBlockRef constructs a new block reference.
func NewBlockRef(hash *hash.Hash) *BlockRef {
	return &BlockRef{Hash: hash}
}

// BuildBlockRef builds a block ref from put opts by hashing the data.
// If putOpts are nil, uses default hash type.
func BuildBlockRef(data []byte, putOpts *PutOpts) (*BlockRef, error) {
	hashType := putOpts.GetHashType()
	if hashType == hash.HashType_HashType_UNKNOWN {
		hashType = defaultHashType
	}
	h, err := hashType.Sum(data)
	if err != nil {
		return nil, err
	}
	return &BlockRef{Hash: hash.NewHash(hashType, h)}, nil
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

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (b *BlockRef) MarshalBlock() ([]byte, error) {
	return proto.Marshal(b)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (b *BlockRef) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, b)
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (b *BlockRef) ApplyBlockRef(id uint32, ptr *BlockRef) error {
	switch id {
	case 1:
		if h := ptr.GetHash(); h != nil {
			b.Hash = proto.Clone(h).(*hash.Hash)
		} else {
			b.Hash = nil
		}
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (b *BlockRef) GetBlockRefs() (map[uint32]*BlockRef, error) {
	return nil, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (b *BlockRef) GetBlockRefCtor(id uint32) Ctor {
	return nil
}

// LessThan checks if the ref is less than another.
// 1. Empty is sorted to the end.
// 2. Hash type is sorted.
// 3. Hash itself is sorted in bytes ordering
func (b *BlockRef) LessThan(other *BlockRef) bool {
	if b.GetEmpty() {
		return false
	}
	if other.GetEmpty() {
		return true
	}
	bh := b.GetHash()
	oh := other.GetHash()
	bht := bh.GetHashType()
	oht := oh.GetHashType()
	if bht != oht {
		return bht < oht
	}
	return bytes.Compare(bh.GetHash(), oh.GetHash()) < 0
}

// UnmarshalBlockRefString unmarshals a string block ref.
func UnmarshalBlockRefString(ref string) (*BlockRef, error) {
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

// _ is a type assertion
var (
	_ Block         = ((*BlockRef)(nil))
	_ BlockWithRefs = ((*BlockRef)(nil))
)
