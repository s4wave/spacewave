package kvtx

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// OpenListRebuild starts the typed walk of the settle of the Volume's old
// generation with opts, as block_gc.ListRebuild describes, on a Volume no
// adoption is counting whose old generation is Open, or a dry reverify on
// any Volume.
func (v *Volume) OpenListRebuild(ctx context.Context, opts block_gc.ListRebuildOptions) (*block_gc.ListRebuild, error) {
	return block_gc.OpenListRebuild(block.WithBackground(ctx), collectorStore{v: v}, opts)
}
