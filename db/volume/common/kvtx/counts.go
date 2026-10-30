package kvtx

import (
	"context"
	"strconv"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
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

// countAuditBlocks bounds the blocks with holders one audit share sums, about
// 100 MB of sums; a store with more audits in more shares.
const countAuditBlocks = 1 << 20

// AuditCounts audits share of shares of the reference counts in one read
// snapshot, as block_gc.RefGraph.AuditCounts describes, and puts the counts in
// doubt when it finds a mismatch, so nothing is freed by them. It reads as
// background work, and returns block_gc.ErrCountAuditFull when the share is
// too large, which more shares fix.
func (v *Volume) AuditCounts(ctx context.Context, share, shares uint32) (block_gc.CountAudit, error) {
	// Audit the share in one read snapshot.
	ctx = block.WithBackground(ctx)
	audit, err := v.readCountAudit(ctx, share, shares)
	if err != nil || audit.MismatchCount == 0 {
		return audit, err
	}

	// Put the counts in doubt for the first mismatch.
	reason := "audit found " + strconv.Itoa(audit.MismatchCount) + " mismatches, first " + audit.Mismatches[0].String()
	return audit, v.withDirectAtomic(ctx, func(_ block.StoreOps, rg *block_gc.RefGraph) error {
		return rg.DistrustCounts(ctx, reason)
	})
}

// readCountAudit audits share of shares of the counts in one read transaction,
// joined with the Volume's direct operations so it cannot outlive Close.
func (v *Volume) readCountAudit(ctx context.Context, share, shares uint32) (block_gc.CountAudit, error) {
	// Join Volume operations while reading.
	v.directMu.RLock()
	defer v.directMu.RUnlock()
	if v.directClosed {
		return block_gc.CountAudit{}, block.ErrPublicationClosed
	}

	// Audit through a reference graph scoped to one read transaction.
	tx, err := v.kvtxStore.NewTransaction(ctx, false)
	if err != nil {
		return block_gc.CountAudit{}, err
	}
	defer tx.Discard()
	return openRefGraph(kvtx.NewTxStore(tx)).AuditCounts(ctx, share, shares, countAuditBlocks)
}
