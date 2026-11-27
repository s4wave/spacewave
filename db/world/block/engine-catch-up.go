package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
)

// catchUpSubscriber resolves a retained span to the worker's captured base.
// Registration precedes the capture, so handOut covers every later advance.
func (e *Engine) catchUpSubscriber(ctx context.Context, w *engineChangeWatch, sub *changeSubscriber, afterSeqno uint64) error {
	// Capture the base and its evidence without publication authority.
	locked := e.changeBcast.Lock()
	if e.feedClosed {
		locked.Unlock()
		return e.closedErr()
	}
	seqno := w.baseSeqno
	plan := watchSpanPlan{unknown: true}
	if after := e.watchedHeadLocked(afterSeqno); after != nil {
		plan = e.watchSpanPlanLocked(after.publication, after.ref, w.basePublication)
	}
	locked.Unlock()

	// Merge outside either shared lock, respecting caller cancellation.
	if err := ctx.Err(); err != nil {
		return err
	}
	sub.add(seqno, resolveWatchSpan(plan))
	return nil
}

// resolveWatchSpan materializes and merges only proven transition summaries.
func resolveWatchSpan(plan watchSpanPlan) *world.ChangeSet {
	// Missing evidence is Unknown, never empty, including after restart.
	if plan.unknown {
		return world.NewUnknownChangeSet()
	}

	// Reuse an adjacent transition's immutable summary without copying its names.
	if len(plan.recorded) == 1 {
		return plan.recorded[0].materialize()
	}

	// A union preserves intermediate changes even when endpoint trees agree.
	var changes *world.ChangeSet
	for _, summary := range plan.recorded {
		changes = world.MergeChangeSets(changes, summary.materialize(), changeDiffLimit)
		if changes.Unknown {
			return changes
		}
	}
	return changes
}
