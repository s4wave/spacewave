package bloom

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/bitset"
	bbloom "github.com/bits-and-blooms/bloom/v3"
	"github.com/golang/protobuf/proto"
)

// NewBloom constructs a new bloom object from an existing bloom.
// if input is nil, returns nil
func NewBloom(bl *bbloom.BloomFilter) *BloomFilter {
	if bl == nil {
		return nil
	}
	return &BloomFilter{
		K:      uint32(bl.K()),
		M:      uint32(bl.Cap()),
		BitSet: bitset.NewBitset(bl.BitSet()),
	}
}

// NewBloomBlock constructs a new Bloom block.
func NewBloomBlock() block.Block {
	return &BloomFilter{}
}

// IsEmpty checks if the bloom filter is empty.
func (b *BloomFilter) IsEmpty() bool {
	m := b.GetM()
	k := b.GetK()
	return b == nil || k == 0 || m == 0 || len(b.GetBitSet().GetSet()) == 0
}

// Clone clones the bloom filter block.
func (b *BloomFilter) Clone() *BloomFilter {
	if b == nil {
		return nil
	}
	return &BloomFilter{
		K:      b.K,
		M:      b.M,
		BitSet: b.GetBitSet().Clone(),
	}
}

// ToBloomFilter converts the bloom block into a BloomFilter.
// Returns nil if empty.
func (b *BloomFilter) ToBloomFilter() *bbloom.BloomFilter {
	if b.IsEmpty() {
		return nil
	}

	m := uint(b.GetM())
	k := uint(b.GetK())
	return bbloom.FromWithM(b.GetBitSet().GetSet(), m, k)
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (b *BloomFilter) MarshalBlock() ([]byte, error) {
	return proto.Marshal(b)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (b *BloomFilter) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, b)
}

// _ is a type assertion
var _ block.Block = ((*BloomFilter)(nil))
