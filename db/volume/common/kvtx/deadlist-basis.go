package kvtx

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// deadlistBasisCurrent certifies historical fates and temporary holder returns
// only against the same complete, known Volume basis. Rebuilds and incarnation
// erasure invalidate it. Unknown history is abandoned, never refunded later.
func deadlistBasisCurrent(ctx context.Context, rg *block_gc.RefGraph, dead *block.Replacements) (bool, error) {
	// Missing certificates never authorize a refund of historical holders.
	if !dead.CountsComplete {
		return false, nil
	}
	generation, err := rg.GetSharingGeneration(ctx)
	if err != nil || generation != dead.CountingGeneration {
		return false, err
	}
	state, err := rg.GetSharing(ctx)
	return state.Phase.Counting() && !state.Doubt, err
}
