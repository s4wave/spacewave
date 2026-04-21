package bitset

import (
	"github.com/s4wave/spacewave/db/block"
	bb "github.com/bits-and-blooms/bitset"
)

// NewBitset constructs a new bitset from an existing bitset.
// if input is nil, returns nil
func NewBitset(bitSet *bb.BitSet) *BitSet {
	if bitSet == nil {
		return nil
	}
	return &BitSet{
		Len: uint32(bitSet.Len()), //nolint:gosec
		Set: bitSet.Bytes(),
	}
}

// ToBitSet converts the bitset block into a BitSet.
func (b *BitSet) ToBitSet() *bb.BitSet {
	if b == nil || len(b.GetSet()) == 0 {
		return &bb.BitSet{}
	}

	return bb.FromWithLength(uint(b.GetLen()), b.GetSet())
}

// Clone clones the bitset block.
func (b *BitSet) Clone() *BitSet {
	if b == nil {
		return nil
	}
	set := b.GetSet()
	bs := make([]uint64, len(set))
	copy(bs, set)
	return &BitSet{
		Set: bs,
		Len: b.GetLen(),
	}
}

// MarshalBlock marshals the block to binary.
func (b *BitSet) MarshalBlock() ([]byte, error) {
	return b.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (b *BitSet) UnmarshalBlock(data []byte) error {
	return b.UnmarshalVT(data)
}

// _ is a type assertion
var _ block.Block = ((*BitSet)(nil))
