package world_block

import (
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
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
		c := w.findCatchUpLocked(afterSeqno, after.root.GetRef())
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

// startCatchUpLocked starts comparing the recorded head after at afterSeqno
// with the watch base and records the comparison for later subscribers. Close
// joins the comparison like a change watch. The caller must hold bcast.
func (e *Engine) startCatchUpLocked(w *engineChangeWatch, afterSeqno uint64, after *watchedHead) *changeCatchUp {
	// Record the comparison, evicting the oldest at capacity.
	c := &changeCatchUp{
		after:    afterSeqno,
		afterRef: after.root.GetRef().CloneVT(),
		seqno:    w.baseSeqno,
		ref:      w.base.GetRef().CloneVT(),
		done:     make(chan struct{}),
	}
	if len(w.catchUps) == changeWatchHeads {
		w.catchUps = slices.Delete(w.catchUps, 0, 1)
	}
	w.catchUps = append(w.catchUps, c)

	// Compare retained heads outside the lock. Missing history cannot
	// certify a filter, so no reader may sleep on the endpoint diff alone.
	middle, complete := e.watchIntermediatesLocked(after.publication, w.basePublication)
	if !complete {
		c.changes = world.NewUnknownChangeSet()
		close(c.done)
		return c
	}
	before, base := after.root.Clone(), w.base.Clone()
	e.changeWatches++
	go func() {
		// Compare until done or the watch stops.
		e.compareCatchUp(w.ctx, c, before, base, middle)

		// Let Close release the stores only after the comparison ends.
		locked := e.bcast.Lock()
		e.changeWatches--
		locked.Broadcast()
		locked.Unlock()
	}()
	return c
}

// compareCatchUp publishes the shared comparison and releases its endpoints.
func (e *Engine) compareCatchUp(ctx context.Context, c *changeCatchUp, before, after *bucket_lookup.Cursor, middle []*bucket_lookup.Cursor) {
	// Release the endpoints before waking the subscribers.
	defer close(c.done)
	defer before.Release()
	defer after.Release()

	// An unreadable head makes the span unknown, not empty.
	var err error
	c.changes, err = e.compareWatchSpan(ctx, before, after, middle)
	if err != nil && ctx.Err() == nil {
		e.le.WithError(err).Warn("world change comparison failed; reporting unknown changes")
	}
}

// compareWatchSpan unions adjacent changes, including changes that cancel
// out between the endpoints. It takes ownership of the intermediate cursors
// but borrows the endpoints. A head that cannot be read makes the span unknown.
func (e *Engine) compareWatchSpan(ctx context.Context, before, after *bucket_lookup.Cursor, middle []*bucket_lookup.Cursor) (*world.ChangeSet, error) {
	// Release the intermediate cursors even when an old head is gone.
	defer func() {
		for _, head := range middle {
			head.Release()
		}
	}()

	// Pin without waiting for a peer. Keep at most two pins live at once.
	ctx, _ = block.WithoutPeerWait(ctx)
	unpin, err := block.PinRoot(ctx, e.writeBlockStore, before.GetRef().GetRootRef())
	if err != nil {
		return world.NewUnknownChangeSet(), nil
	}
	defer func() { unpin() }()

	// Merge one comparison at a time, taking the next pin before dropping
	// the previous one so the next comparison can use it as its base.
	var changes *world.ChangeSet
	for i := range len(middle) + 1 {
		next := after
		if i < len(middle) {
			next = middle[i]
		}
		release, err := block.PinRoot(ctx, e.writeBlockStore, next.GetRef().GetRootRef())
		if err != nil {
			return world.NewUnknownChangeSet(), nil
		}
		_, beforeBcs := before.BuildTransactionWithStore(nil, e.writeBlockStore)
		_, nextBcs := next.BuildTransactionWithStore(nil, e.writeBlockStore)
		delta, err := WorldChanges(ctx, beforeBcs, nextBcs, changeDiffLimit)
		unpin()
		before, unpin = next, release
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
