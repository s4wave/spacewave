package block_store_overlay

import (
	"context"
	"time"

	"github.com/aperturerobotics/hydra/block"
)

// OverlayBlock is a block store overlaying two other block stores.
type OverlayBlock = block.StoreOverlay

// NewOverlayBlock builds a new block store on top of two other block stores.
//
// ctx is used for writeback requests
func NewOverlayBlock(
	ctx context.Context,
	lower,
	upper block.StoreOps,
	mode block.OverlayMode,
	writebackTimeout time.Duration,
	writebackPutOpts *block.PutOpts,
) *OverlayBlock {
	return block.NewOverlay(ctx, lower, upper, mode, writebackTimeout, writebackPutOpts)
}

// _ is a type assertion
var _ block.StoreOps = ((*OverlayBlock)(nil))
