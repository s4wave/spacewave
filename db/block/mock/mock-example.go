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

// DecodedBlockCacheTypeKey returns the decoded-block cache type key.
func (e *Example) DecodedBlockCacheTypeKey() string {
	return "db/block/mock.Example"
}

// UnmarshalExample unmarshals the example block.
// Returns nil, nil if empty.
func UnmarshalExample(ctx context.Context, bcs *block.Cursor) (*Example, error) {
	return block.UnmarshalBlock[Example](ctx, bcs)
}

// _ is a type assertion
var (
	_ block.Block                 = (*Example)(nil)
	_ block.DecodedBlockCacheable = (*Example)(nil)
)
