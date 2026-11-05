package kvtx

import (
	"context"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// OpenListRebuild starts a rebuild of the Volume's unverified child lists, as
// block_gc.ListRebuild describes, on a Volume that is not adopted.
func (v *Volume) OpenListRebuild(ctx context.Context) (*block_gc.ListRebuild, error) {
	return block_gc.OpenListRebuild(backgroundPass(ctx), collectorStore{v: v})
}
