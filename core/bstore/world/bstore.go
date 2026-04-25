package bstore_world

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// NewBlockStoreStateBlock constructs a new BlockStoreState block.
func NewBlockStoreStateBlock() block.Block {
	return &BlockStoreState{}
}

// UnmarshalBlockStoreState unmarshals a BlockStoreState from a block cursor.
func UnmarshalBlockStoreState(ctx context.Context, bcs *block.Cursor) (*BlockStoreState, error) {
	return block.UnmarshalBlock[*BlockStoreState](ctx, bcs, NewBlockStoreStateBlock)
}

// Validate validates the BlockStoreState.
func (i *BlockStoreState) Validate() error {
	return nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (i *BlockStoreState) MarshalBlock() ([]byte, error) {
	return i.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (i *BlockStoreState) UnmarshalBlock(data []byte) error {
	return i.UnmarshalVT(data)
}

// _ is a type assertion
var _ block.Block = ((*BlockStoreState)(nil))
