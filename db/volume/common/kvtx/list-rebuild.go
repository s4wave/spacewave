package kvtx

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// OpenListRebuild starts the typed walk of the settle of the Volume's old
// generation, as block_gc.ListRebuild describes, on a Volume no adoption is
// counting whose old generation is Open.
func (v *Volume) OpenListRebuild(ctx context.Context) (*block_gc.ListRebuild, error) {
	return block_gc.OpenListRebuild(block.WithBackground(ctx), collectorStore{v: v})
}

// OpenListCheck starts a dry run of the list rebuild on the Volume in any
// state, as block_gc.OpenListCheck describes.
func (v *Volume) OpenListCheck() *block_gc.ListRebuild {
	return block_gc.OpenListCheck(collectorStore{v: v})
}
