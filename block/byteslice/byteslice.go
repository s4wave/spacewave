package byteslice

import "github.com/aperturerobotics/hydra/block"

// ByteSlice is a byte slice sub-block.
type ByteSlice struct {
	sl *[]byte
}

// NewByteSlice constructs a new sub-block from a byte slice.
//
// If sl is nil, returns nil.
func NewByteSlice(sl *[]byte) *ByteSlice {
	if sl == nil {
		return nil
	}
	return &ByteSlice{sl: sl}
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (b *ByteSlice) MarshalBlock() ([]byte, error) {
	if b == nil || b.sl == nil {
		return nil, nil
	}
	sl := *b.sl
	d := make([]byte, len(sl))
	copy(d, sl)
	return d, nil
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (b *ByteSlice) UnmarshalBlock(data []byte) error {
	if b != nil && b.sl != nil {
		*b.sl = data
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block    = ((*ByteSlice)(nil))
	_ block.SubBlock = ((*ByteSlice)(nil))
)
