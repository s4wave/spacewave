package byteslice

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// ByteSlice is a byte slice sub-block.
type ByteSlice struct {
	alias block.AliasIdentityToken
	sl    *[]byte
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
// If the cursor or byte slice is empty & apply is set, sets a empty ref.
// If apply is set, updates the block cursor to hold a BlockRef object.
func ByteSliceToRef(ctx context.Context, bcs *block.Cursor, apply bool) (*block.BlockRef, error) {
	nodRefi, _ := bcs.GetBlock()
	if nodRefi == nil && !apply {
		return &block.BlockRef{}, nil
	}
	if nr, ok := nodRefi.(*ByteSlice); ok && nr != nil {
		br := &block.BlockRef{}
		if len(nr.GetBytes()) != 0 {
			if err := br.UnmarshalBlock(nr.GetBytes()); err != nil {
				return nil, err
			}
			if err := br.Validate(false); err != nil {
				return nil, err
			}
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

// BlockAliasIdentity returns the in-memory alias token for ByteSlice.
func (b *ByteSlice) BlockAliasIdentity() *block.AliasIdentityToken {
	return &b.alias
}

// GetBytes returns the byte slice.
func (b *ByteSlice) GetBytes() []byte {
	if b.sl == nil {
		return nil
	}
	return *b.sl
}

// CloneBlock clones the byte slice block without sharing the backing slice.
func (b *ByteSlice) CloneBlock() (block.Block, error) {
	if b == nil || b.sl == nil {
		return &ByteSlice{}, nil
	}
	sl := *b.sl
	clone := make([]byte, len(sl))
	copy(clone, sl)
	return &ByteSlice{sl: &clone}, nil
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
	_ block.Block          = ((*ByteSlice)(nil))
	_ block.BlockWithClone = ((*ByteSlice)(nil))
	_ block.SubBlock       = ((*ByteSlice)(nil))
)
