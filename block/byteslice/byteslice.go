package byteslice

import (
	"context"

	"github.com/aperturerobotics/hydra/block"
)

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

// NewByteSliceBlock constructs a new byte slice block.
func NewByteSliceBlock() block.Block {
	return &ByteSlice{}
}

// ByteSliceToRef converts a byte slice cursor into a block.BlockRef.
// If the cursor is empty & apply is set, sets a empty ref.
// If apply is set, updates the block cursor to hold a BlockRef object.
func ByteSliceToRef(ctx context.Context, bcs *block.Cursor, apply bool) (*block.BlockRef, error) {
	nodRefi, _ := bcs.GetBlock()
	if nodRefi == nil && !apply {
		return &block.BlockRef{}, nil
	}
	if nr, ok := nodRefi.(*ByteSlice); ok && nr != nil {
		br := &block.BlockRef{}
		if err := br.UnmarshalBlock(nr.GetBytes()); err != nil {
			return nil, err
		}
		if err := br.Validate(); err != nil {
			return nil, err
		}
		if apply {
			bcs.SetBlock(br, false)
		}
	}

	return block.UnmarshalBlockRefBlock(ctx, bcs)
}

// IsNil returns if the object is nil.
func (b *ByteSlice) IsNil() bool {
	return b == nil
}

// GetBytes returns the byte slice.
func (b *ByteSlice) GetBytes() []byte {
	if b.sl == nil {
		return nil
	}
	return *b.sl
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
	} else {
		m := make([]byte, len(data))
		copy(m, data)
		b.sl = &m
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block    = ((*ByteSlice)(nil))
	_ block.SubBlock = ((*ByteSlice)(nil))
)
