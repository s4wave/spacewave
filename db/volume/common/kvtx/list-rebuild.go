package kvtx

import (
	"context"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// OpenListRebuild starts the typed walk of the settle of the Volume's old
// generation, as block_gc.ListRebuild describes, on a Volume no adoption is
// counting whose old generation is Open.
func (v *Volume) OpenListRebuild(ctx context.Context) (*block_gc.ListRebuild, error) {
	return block_gc.OpenListRebuild(backgroundPass(ctx), collectorStore{v: v})
}
