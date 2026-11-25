package world_block

import (
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
)

// changeCatchUp is one comparison from a recorded head to a base of the
// change watch, shared by the subscribers that start behind the watch at
// that head.
type changeCatchUp struct {
	// after is the seqno of the recorded head the comparison starts at.
	after uint64
	// afterRef is the reference of the recorded head.
	afterRef *bucket.ObjectRef
	// seqno is the seqno of the watch base the comparison reaches, where a
	// recorded span starts.
	seqno uint64
	// ref is the reference of the watch base.
	ref *bucket.ObjectRef
	// done closes once changes is set.
	done chan struct{}
	// changes lists the changes between the two heads, set before done
	// closes.
	changes *world.ChangeSet
}

// catchUpSubscriber merges into sub the changes from the recorded head at
// afterSeqno to the change watch's base, which the spans the watch records
// continue from. The subscribers that start at the same head share one
// comparison, so a reconnect wave compares each head its clients held once,
// not once per subscriber.
func (e *Engine) catchUpSubscriber(ctx context.Context, w *engineChangeWatch, sub *changeSubscriber, afterSeqno uint64) error {
	for {
		// Join the comparison from the head at afterSeqno, or start one to
		// the base. Without a single recorded head there, the changes up to
		// the base are unknown.
		locked := e.bcast.Lock()
		if e.closed {
			locked.Unlock()
			return e.closedErr()
		}
		after := e.watchedHeadLocked(afterSeqno)
		if after == nil {
			seqno := w.baseSeqno
			locked.Unlock()
			sub.add(seqno, world.NewUnknownChangeSet())
			return nil
		}
		c := w.findCatchUpLocked(afterSeqno, after.ref)
		if c == nil {
			c = e.startCatchUpLocked(w, afterSeqno, after)
		}
		locked.Unlock()

		// Wait for the comparison.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.done:
		}

		// Continue from the comparison's base with the spans the watch
		// recorded since. A comparison the history no longer reaches is
		// dropped and compared again to the current base.
		locked = e.bcast.Lock()
		caughtUp := c.seqno >= w.baseSeqno
		var spans []changeSpan
		if !caughtUp {
			spans, caughtUp = w.spansSinceLocked(c.seqno, c.ref)
		}
		if !caughtUp {
			w.dropCatchUpLocked(c)
		}
		baseSeqno := w.baseSeqno
		locked.Unlock()
		if !caughtUp {
			continue
		}

		// Merge the shared comparison and the spans after it.
		sub.add(c.seqno, c.changes)
		if len(spans) != 0 {
			sub.add(baseSeqno, mergeSpans(spans))
		}
		return nil
	}
}

// startCatchUpLocked starts resolving the changes from the recorded head after
// at afterSeqno to the watch base and records the catch-up for later
// subscribers. Close joins the resolution like a change watch. The caller must
// hold bcast.
func (e *Engine) startCatchUpLocked(w *engineChangeWatch, afterSeqno uint64, after *watchedHead) *changeCatchUp {
	// Record the catch-up, evicting the oldest at capacity.
	c := &changeCatchUp{
		after:    afterSeqno,
		afterRef: after.ref.CloneVT(),
		seqno:    w.baseSeqno,
		ref:      w.base.GetRef().CloneVT(),
		done:     make(chan struct{}),
	}
	if len(w.catchUps) == changeWatchHeads {
		w.catchUps = slices.Delete(w.catchUps, 0, 1)
	}
	w.catchUps = append(w.catchUps, c)

	// Collect the span plan under bcast. Complete immediately when unknown or
	// one recorded ChangeSet covers the span; otherwise merge and compare in
	// one shared goroutine outside bcast.
	plan := e.watchSpanPlanLocked(after.publication, after.ref, w.basePublication)
	if plan.unknown {
		c.changes = world.NewUnknownChangeSet()
		close(c.done)
		return c
	}
	if len(plan.pairs) == 0 && len(plan.recorded) == 1 {
		c.changes = plan.recorded[0]
		close(c.done)
		return c
	}
	e.changeWatches++
	go func() {
		// Resolve the span until done or the watch stops.
		e.resolveCatchUp(w.ctx, c, plan)

		// Let Close release the stores only after resolution ends.
		locked := e.bcast.Lock()
		e.changeWatches--
		locked.Broadcast()
		locked.Unlock()
	}()
	return c
}

// resolveCatchUp publishes the shared catch-up ChangeSet and wakes its waiters.
func (e *Engine) resolveCatchUp(ctx context.Context, c *changeCatchUp, plan watchSpanPlan) {
	defer close(c.done)

	// An unreadable fallback head makes the span unknown, not empty.
	var err error
	c.changes, err = e.resolveWatchSpan(ctx, plan)
	if err != nil && ctx.Err() == nil {
		e.le.WithError(err).Warn("world change comparison failed; reporting unknown changes")
	}
}

// resolveWatchSpan merges the recorded ChangeSets of plan and falls back to
// tree comparison for each unrecorded adjacent pair, stopping once the union
// becomes unknown.
func (e *Engine) resolveWatchSpan(ctx context.Context, plan watchSpanPlan) (*world.ChangeSet, error) {
	// Return unknown immediately for an incomplete or overflowing plan.
	if plan.unknown {
		return world.NewUnknownChangeSet(), nil
	}

	// Merge recorded ChangeSets before touching storage.
	var changes *world.ChangeSet
	for _, cs := range plan.recorded {
		changes = world.MergeChangeSets(changes, cs, changeDiffLimit)
		if changes.Unknown {
			return changes, nil
		}
	}

	// Fall back to tree comparison for any publication without a recorded
	// ChangeSet while its pages remain readable.
	if len(plan.pairs) != 0 {
		compared, err := e.compareRefPairs(ctx, plan.pairs)
		if err != nil {
			return world.NewUnknownChangeSet(), err
		}
		changes = world.MergeChangeSets(changes, compared, changeDiffLimit)
	}
	if changes == nil {
		return &world.ChangeSet{}, nil
	}
	return changes, nil
}

// compareRefPairs compares each adjacent head ref pair and unions their
// ChangeSets, pinning at most two roots at once without waiting for an absent
// peer. An unreadable root makes the span unknown.
func (e *Engine) compareRefPairs(ctx context.Context, pairs []watchedRefPair) (*world.ChangeSet, error) {
	// Pin without waiting for a peer, keeping at most two pins live at once.
	ctx, _ = block.WithoutPeerWait(ctx)
	var pinnedRef *block.BlockRef
	var unpin func()
	defer func() {
		if unpin != nil {
			unpin()
		}
	}()

	// Compare each pair, reusing the previous after pin for consecutive pairs.
	var changes *world.ChangeSet
	for _, pair := range pairs {
		if pinnedRef == nil || !pinnedRef.EqualsRef(pair.before.GetRootRef()) {
			if unpin != nil {
				unpin()
				unpin = nil
			}
			var err error
			unpin, err = block.PinRoot(ctx, e.writeBlockStore, pair.before.GetRootRef())
			if err != nil {
				return world.NewUnknownChangeSet(), nil
			}
		}
		nextUnpin, err := block.PinRoot(ctx, e.writeBlockStore, pair.after.GetRootRef())
		if err != nil {
			return world.NewUnknownChangeSet(), nil
		}
		delta, err := e.compareObjectRefs(ctx, pair.before, pair.after)
		unpin()
		pinnedRef, unpin = pair.after.GetRootRef(), nextUnpin
		if err != nil {
			return world.NewUnknownChangeSet(), err
		}
		changes = world.MergeChangeSets(changes, delta, changeDiffLimit)
		if changes.Unknown {
			break
		}
	}
	return changes, nil
}

// compareObjectRefs follows two pinned ObjectRefs and lists their WorldChanges.
func (e *Engine) compareObjectRefs(ctx context.Context, before, after *bucket.ObjectRef) (*world.ChangeSet, error) {
	// Follow both pinned refs on the base bucket cursor.
	beforeCur, err := e.baseRoot.FollowRef(ctx, before)
	if err != nil {
		return nil, err
	}
	defer beforeCur.Release()
	afterCur, err := e.baseRoot.FollowRef(ctx, after)
	if err != nil {
		return nil, err
	}
	defer afterCur.Release()

	// Compare their object and graph trees.
	_, beforeBcs := beforeCur.BuildTransactionWithStore(nil, e.writeBlockStore)
	_, afterBcs := afterCur.BuildTransactionWithStore(nil, e.writeBlockStore)
	return WorldChanges(ctx, beforeBcs, afterBcs, changeDiffLimit)
}

// findCatchUpLocked returns the recorded comparison from the head at after,
// or nil. The caller must hold bcast.
func (w *engineChangeWatch) findCatchUpLocked(after uint64, afterRef *bucket.ObjectRef) *changeCatchUp {
	i := slices.IndexFunc(w.catchUps, func(c *changeCatchUp) bool {
		return c.after == after && c.afterRef.EqualsRef(afterRef)
	})
	if i < 0 {
		return nil
	}
	return w.catchUps[i]
}

// dropCatchUpLocked forgets c so the next subscriber compares again. The
// caller must hold bcast.
func (w *engineChangeWatch) dropCatchUpLocked(c *changeCatchUp) {
	w.catchUps = slices.DeleteFunc(w.catchUps, func(o *changeCatchUp) bool {
		return o == c
	})
}
