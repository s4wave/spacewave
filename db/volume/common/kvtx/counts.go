package kvtx

import (
	"context"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// countBuildChunkStart is the records the first slice of a count build reads.
const countBuildChunkStart = 64

// countBuildChunkSize bounds the records one slice of a count build reads,
// once slices have grown from countBuildChunkStart. A record costs a read and
// a few count writes, far less than a swept node.
const countBuildChunkSize = 4096

// BuildCounts builds the reference counts of a store that tracked blocks
// before it kept them, for up to budget, as block_gc.RefGraph.BuildCounts
// describes, and returns the counts' state; a building state has more to do. Each slice is one
// write transaction sized to take about sweepChunkTime, which yields to a
// waiting writer after its first record, and reads as background work so the
// walk stays out of the page cache.
func (v *Volume) BuildCounts(ctx context.Context, budget time.Duration) (block_gc.CountState, error) {
	ctx = block.WithBackground(ctx)
	end := time.Now().Add(budget)
	size := countBuildChunkStart
	for {
		// Run one slice.
		start := time.Now()
		var state block_gc.CountState
		err := v.withDirectAtomic(ctx, func(blocks block.StoreOps, rg *block_gc.RefGraph) error {
			var err error
			state, err = rg.BuildCounts(ctx, size, v.writersWaiting, blocks.GetBlockExists)
			return err
		})
		if err != nil || !state.Building() {
			return state, err
		}

		// Stop at the budget, or size the next slice.
		if time.Now().After(end) {
			return state, nil
		}
		size = nextChunkSize(size, time.Since(start), countBuildChunkSize)
	}
}
