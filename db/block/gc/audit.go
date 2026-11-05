package block_gc

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// auditSampleSize bounds the blocks an audit lists by name for each finding.
const auditSampleSize = 8

// AuditResult compares what the named roots reach with what the collector
// would keep. It is an audit: it frees nothing.
type AuditResult struct {
	// Mark is what the walk from the roots reached.
	Mark *Mark
	// Stored counts the stored blocks, which have a child list.
	Stored uint64
	// Leaked counts the stored blocks no root reaches that the collector has
	// not queued. After a pass that left nothing to do, that is garbage a
	// hole, a put outside a batch or a lost release keeps; during a cascade
	// it also counts the children of blocks still queued.
	Leaked uint64
	// LiveQueued counts the blocks a root reaches that sit on the free queue
	// with no journal record past the cursor adding a holder: blocks the
	// collector would free while they are live. Any is an error in the
	// counts.
	LiveQueued uint64
	// LiveQueuedSample names up to auditSampleSize of them.
	LiveQueuedSample []*block.BlockRef
}

// Audit walks from the named roots of the Volume in one snapshot and compares
// the walk with the counts and the free queue, as AuditResult describes. It
// keeps the reached set in memory, so it suits a copy host.
func Audit(ctx context.Context, store AdoptStore) (AuditResult, error) {
	var res AuditResult
	err := store.View(ctx, func(rec *Records) error {
		// Walk from the roots.
		mark, err := Reachable(ctx, rec, store.BlockStored)
		if err != nil {
			return err
		}
		res.Mark = mark

		// Read the blocks the journal past the cursor adds a holder to.
		state, err := rec.GetState(ctx)
		if err != nil {
			return err
		}
		tail := make(map[string]struct{})
		err = rec.ScanJournal(ctx, state.GetCursor(), func(_ uint64, r *JournalRecord) (bool, error) {
			for _, up := range r.GetUp() {
				tail[string(up)] = struct{}{}
			}
			return true, nil
		})
		if err != nil {
			return err
		}

		// Compare each stored block's liveness with the queue.
		return rec.ScanChildren(ctx, func(ref *block.BlockRef, _ []*block.BlockRef) error {
			// Read whether the block is queued and the tail holds it.
			res.Stored++
			queued, err := rec.Queued(ctx, ref)
			if err != nil {
				return err
			}
			_, held := tail[string(BlockKey(ref))]

			// Count a live block the collector would free, and a leak.
			switch {
			case mark.Has(ref) && queued && !held:
				res.LiveQueued++
				if len(res.LiveQueuedSample) < auditSampleSize {
					res.LiveQueuedSample = append(res.LiveQueuedSample, ref)
				}
			case !mark.Has(ref) && !queued:
				res.Leaked++
			}
			return nil
		})
	})
	return res, err
}
