package bitset

import (
	"github.com/aperturerobotics/hydra/block"
	bb "github.com/bits-and-blooms/bitset"
	"google.golang.org/protobuf/proto"
)

// NewBitset constructs a new bitset from an existing bitset.
// if input is nil, returns nil
func NewBitset(bitSet *bb.BitSet) *BitSet {
	if bitSet == nil {
		return nil
	}
	return &BitSet{
		Len: uint32(bitSet.Len()),
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
// This is the initial step of marshaling, before transformations.
func (b *BitSet) MarshalBlock() ([]byte, error) {
	return proto.Marshal(b)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (b *BitSet) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, b)
}

// _ is a type assertion
var _ block.Block = ((*BitSet)(nil))
