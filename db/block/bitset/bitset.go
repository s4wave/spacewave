package bitset

import "github.com/s4wave/spacewave/db/block"

// NewBitset constructs a new bitset from existing words.
func NewBitset(words []uint64, length uint32) *BitSet {
	if len(words) == 0 {
		return nil
	}
	return &BitSet{
		Len: length,
		Set: words,
	}
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
