package kvtx

import (
	"context"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// OpenListRebuild starts a rebuild of the Volume's unverified child lists, as
// block_gc.ListRebuild describes, on a Volume no adoption is counting. A
// Finish that returns the Volume to unadopted wakes the background adoption,
// which recounts it when the collector frees.
func (v *Volume) OpenListRebuild(ctx context.Context) (*block_gc.ListRebuild, error) {
	return block_gc.OpenListRebuild(backgroundPass(ctx), collectorStore{v: v}, v.wakeAdopt)
}
