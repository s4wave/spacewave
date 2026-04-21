package filters

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/bloom"
)

// NewKeyFiltersBlock constructs a new KeyFilters block.
func NewKeyFiltersBlock() block.Block {
	return &KeyFilters{}
}

// UnmarshalKeyFilters unmarshals a world change ll from a cursor.
// If empty, returns nil, nil
func UnmarshalKeyFilters(ctx context.Context, bcs *block.Cursor) (*KeyFilters, error) {
	return block.UnmarshalBlock[*KeyFilters](ctx, bcs, NewKeyFiltersBlock)
}

// IsNil returns if the object is nil.
func (w *KeyFilters) IsNil() bool {
	return w == nil
}

// IsEmpty checks if the world change is empty.
func (w *KeyFilters) IsEmpty() bool {
	return w.GetKeyBloom().IsEmpty() &&
		w.GetKeyPrefix() == "" &&
		w.GetQuadPrefix().IsEmpty()
}

// Clone clones the key filters object.
func (w *KeyFilters) Clone() *KeyFilters {
	if w == nil {
		return nil
	}
	return &KeyFilters{
		KeyPrefix:  w.GetKeyPrefix(),
		QuadPrefix: w.GetQuadPrefix().Clone(),
		KeyBloom:   w.GetKeyBloom().Clone(),
	}
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (w *KeyFilters) MarshalBlock() ([]byte, error) {
	return w.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (w *KeyFilters) UnmarshalBlock(data []byte) error {
	return w.UnmarshalVT(data)
}

// ApplySubBlock applies a sub-block change with a field id.
func (w *KeyFilters) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 3:
		v, ok := next.(*bloom.BloomFilter)
		if !ok {
			return block.ErrUnexpectedType
		}
		w.KeyBloom = v
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (w *KeyFilters) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	m[3] = w.GetKeyBloom()
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (w *KeyFilters) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 3:
		return func(create bool) block.SubBlock {
			v := w.GetKeyBloom()
			if v == nil && create {
				w.KeyBloom = &bloom.BloomFilter{}
				v = w.KeyBloom
			}
			return v
		}
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*KeyFilters)(nil))
	_ block.BlockWithSubBlocks = ((*KeyFilters)(nil))
)
