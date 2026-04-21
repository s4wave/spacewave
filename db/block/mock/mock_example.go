package block_mock

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// NewExample builds a new example block with a message.
func NewExample(msg string) *Example {
	return &Example{Msg: msg}
}

// NewExampleBlock builds a new example block.
func NewExampleBlock() block.Block {
	return &Example{}
}

// UnmarshalExample unmarshals the example block.
// Returns nil, nil if empty
func UnmarshalExample(ctx context.Context, bcs *block.Cursor) (*Example, error) {
	return block.UnmarshalBlock[*Example](ctx, bcs, NewExampleBlock)
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (e *Example) MarshalBlock() ([]byte, error) {
	return e.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (e *Example) UnmarshalBlock(data []byte) error {
	return e.UnmarshalVT(data)
}

// _ is a type assertion
var _ block.Block = ((*Example)(nil))
