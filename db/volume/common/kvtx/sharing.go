package kvtx

import (
	"context"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// walkSharing runs the graph's sharing walk to its end in budgeted
// transactions, as RefGraph.WalkSharing describes, each yielding the writer
// after its budget or to a waiting writer. A cancelled walk keeps its place
// for the next block pass.
func (v *Volume) walkSharing(ctx context.Context) error {
	if !v.holdCounts {
		return nil
	}
	for {
		// Walk one slice.
		var s block_gc.SharingState
		err := v.withDirectAtomic(ctx, func(_ block.StoreOps, rg *block_gc.RefGraph) (err error) {
			held := time.Now()
			yield := func() bool { return time.Since(held) >= sweepBudget || v.writersWaiting() }
			s, err = rg.WalkSharing(ctx, yield)
			return err
		})
		if err != nil {
			return err
		}
		if s.Phase.Running() {
			continue
		}

		// Report a walk in doubt, whose counts may be short.
		if s.Doubt && v.le != nil {
			v.le.Warn("shared counts are in doubt: the sharing walk met a holder it cannot count")
		}
		return nil
	}
}
