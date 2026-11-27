package world_block

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
)

// changeWatchHeads bounds the published heads the Engine records so a new
// change subscriber can catch up from the head it last saw against a later one.
const changeWatchHeads = 128

// watchedHead is a published head recorded for change watchers. It holds only
// the head's ref and ChangeSet, never a cursor, so past Worlds stay collectible.
type watchedHead struct {
	// before identifies the exact predecessor root.
	before *block.BlockRef
	// seqno is the World seqno of the head.
	seqno uint64
	// publication is the local publication number, independent of the seqno.
	publication uint64
	// ref is the ObjectRef of the head.
	ref *bucket.ObjectRef
	// changes is the ChangeSet from the preceding publication to this head,
	// or nil when the head arrived without one.
	changes *changeSummary
}

// watchSpanPlan holds immutable summaries for a proven contiguous span.
type watchSpanPlan struct {
	// recorded holds detached summaries in publication order.
	recorded []*changeSummary
	// unknown reports missing or overwritten transition evidence.
	unknown bool
}

// changeSubscriber is one WatchChanges call on the change watch. Its pending
// changes have their own lock, so merging them never holds the Engine bcast.
type changeSubscriber struct {
	// interest selects the changes that wake the subscriber. A subscriber a
	// WatchSlot keeps swaps it on each call, while the change watch reads it.
	interest atomic.Pointer[world.ChangeInterest]
	// wake receives a value when changes is set or the watch loses track.
	wake chan struct{}

	// mtx guards the fields below.
	mtx sync.Mutex
	// idle is set while a WatchSlot keeps the subscriber between calls. The
	// subscriber merges no changes then, since the next call catches up from
	// its own afterSeqno.
	idle bool
	// seqno is the seqno of the newest head merged into changes.
	seqno uint64
	// changes merges the changes not yet reported, or is nil.
	changes *world.ChangeSet
}

// slotSubscriber is a changeSubscriber a WatchSlot keeps between the calls of
// one watcher, still joined to its change watch and indexed by the interest of
// its last call.
type slotSubscriber struct {
	// w is the change watch sub joined.
	w *engineChangeWatch
	// sub is the kept subscriber.
	sub *changeSubscriber
}

// ReleaseWatch leaves the change watch.
func (s *slotSubscriber) ReleaseWatch() {
	s.w.leave(s.sub)
}

// WatchChanges calls cb with later revisions whose changes touches accepts.
// Calls coalesce while cb runs. cb must not modify the shared ChangeSet.
// Retained exact transition summaries preserve intermediate changes, including
// changes that cancel between the endpoints. A missing summary, overwritten
// notice, ambiguous revision or overflowing union yields Unknown so readers
// refresh. Notification delivery never reads the object or graph trees.
func (e *Engine) WatchChanges(
	ctx context.Context,
	afterSeqno uint64,
	touches world.ChangeFilter,
	cb func(seqno uint64, changes *world.ChangeSet) error,
) error {
	return e.WatchIndexedChanges(ctx, afterSeqno, world.NewChangeInterest(touches), cb)
}

// WatchIndexedChanges is WatchChanges for the changes interest accepts. The
// change watch indexes the subscriber by the names in interest, so a change
// referencing none of those names never tests or wakes the subscriber.
func (e *Engine) WatchIndexedChanges(
	ctx context.Context,
	afterSeqno uint64,
	interest *world.ChangeInterest,
	cb func(seqno uint64, changes *world.ChangeSet) error,
) error {
	// Subscribe to the change watch, which keeps it running until the
	// subscriber leaves.
	w, sub, done, err := e.subscribeChanges(ctx, interest)
	if err != nil {
		return err
	}
	var covered uint64
	defer func() { done(covered) }()

	// Copy the recorded spans when the change watch starts past afterSeqno and
	// recorded every span since the head at afterSeqno. A reconnect wave
	// subscribes thousands of watchers at once, so the merge runs unlocked.
	locked := e.changeBcast.Lock()
	caughtUp := afterSeqno >= w.baseSeqno
	var spans []changeSpan
	if base := e.watchedHeadLocked(afterSeqno); !caughtUp && base != nil {
		spans, caughtUp = w.spansSinceLocked(afterSeqno, base.ref)
	}
	baseSeqno := w.baseSeqno
	locked.Unlock()

	// Merge recorded spans or the retained transition summaries from afterSeqno. The first wake checks whether the watch already ended.
	if len(spans) != 0 {
		sub.add(baseSeqno, mergeSpans(spans))
	}
	if !caughtUp {
		if err := e.catchUpSubscriber(ctx, w, sub, afterSeqno); err != nil {
			return err
		}
	}
	sub.signal()

	// Report each change past afterSeqno.
	for {
		// Take the merged changes, or wait for the next. A wake that brings
		// no changes may report that the watch ended.
		seqno, changes := sub.take()
		for changes == nil {
			select {
			case <-ctx.Done():
				covered = sub.coveredThrough(w, afterSeqno)
				return ctx.Err()
			case <-sub.wake:
			}
			if seqno, changes = sub.take(); changes == nil {
				if err := e.changeWatchErr(); err != nil {
					return err
				}
			}
		}

		// Skip changes the caller already saw, and catch-up changes or
		// changes merged before the filter ran that it rejects.
		if seqno <= afterSeqno || !interest.Accepts(changes) {
			continue
		}
		if err := cb(seqno, changes); err != nil {
			return err
		}
		afterSeqno = seqno
	}
}

// subscribeChanges joins the change watch with a subscriber indexed by
// interest, and returns done, which ends the call with the seqno the call
// covered. A subscriber keeps the watch running until the subscriber leaves.
//
// With a WatchSlot in ctx, subscribeChanges reuses the subscriber the slot
// kept from an earlier call on the same watch, emptied of the changes the
// subscriber merged, and moves its index entry by the difference between
// the interests, so a view that reruns after each change touches only the
// names its reads gained or lost. done puts the subscriber back in the slot
// with the seqno the call covered, and the slot leaves the watch when the
// slot closes. The reuse keeps a fresh subscriber's order, indexing before
// the caller reads the watch's base, so the caller's catch-up from
// afterSeqno covers every change the emptied subscriber dropped. Without a
// slot, done leaves the watch.
func (e *Engine) subscribeChanges(ctx context.Context, interest *world.ChangeInterest) (*engineChangeWatch, *changeSubscriber, func(covered uint64), error) {
	// Claim the slot and the subscriber it kept on this Engine.
	slot := world.WatchSlotFromContext(ctx)
	var kept *slotSubscriber
	if slot != nil {
		held, ok := slot.Take()
		if !ok {
			slot = nil
		} else if held != nil {
			if kept, _ = held.(*slotSubscriber); kept == nil || kept.w.bcast != &e.changeBcast {
				held.ReleaseWatch()
				kept = nil
			}
		}
	}

	// Join the running watch, keeping the subscriber when it joined the same
	// one. A stopped watch took its subscribers with it.
	locked := e.changeBcast.Lock()
	w, err := e.startChangeWatchLocked()
	var stale *slotSubscriber
	if kept != nil && kept.w != w {
		stale, kept = kept, nil
	}
	var sub *changeSubscriber
	if err == nil {
		if kept != nil {
			sub = kept.sub
		} else {
			sub = &changeSubscriber{wake: make(chan struct{}, 1)}
			w.addSubLocked(sub)
		}
	}
	locked.Unlock()

	// Release an obsolete subscription before completing this claim.
	if stale != nil {
		stale.ReleaseWatch()
	}
	if err != nil {
		if slot != nil {
			slot.Put(nil, 0)
		}
		return nil, nil, nil, err
	}

	// Empty a kept subscriber of the changes it merged for the last call, so
	// this call reports only what its catch-up and the watch hand it.
	if kept != nil {
		sub.mtx.Lock()
		sub.idle, sub.seqno, sub.changes = false, 0, nil
		sub.mtx.Unlock()
		select {
		case <-sub.wake:
		default:
		}
	}

	// Index the subscriber by its interest's names outside the publication
	// lock, so World writes never wait behind the naming. It indexes before
	// the caller reads the base, so the watch hands it every span it records
	// past that base.
	sub.interest.Store(interest)
	w.indexSub(sub, interest)

	// End the call by keeping the subscriber in the slot, idle, or by leaving.
	if slot == nil {
		return w, sub, func(uint64) { w.leave(sub) }, nil
	}
	return w, sub, func(covered uint64) {
		sub.mtx.Lock()
		sub.idle, sub.changes = true, nil
		sub.mtx.Unlock()
		slot.Put(&slotSubscriber{w: w, sub: sub}, covered)
	}, nil
}

// watchedHeadLocked returns an unambiguous retained revision, or nil.
// The caller holds feed authority; ring slots are read atomically.
func (e *Engine) watchedHeadLocked(seqno uint64) *watchedHead {
	// Scan a bounded ring, rejecting evicted or ambiguous starting revisions.
	if seqno < e.watchedFloor.Load() {
		return nil
	}
	latest := e.latestNotice.Load()
	var found *watchedHead
	for i := range e.watchedHeads {
		h := e.watchedHeads[i].Load()
		if h == nil || h.seqno != seqno {
			continue
		}
		if latest.publication-h.publication >= changeWatchHeads {
			return nil
		}
		if found != nil && !found.ref.EqualsRef(h.ref) {
			return nil
		}
		found = h
	}
	return found
}

// recordInitialHead records the initial revision before the Engine is shared.
func (e *Engine) recordInitialHead(ctx context.Context) error {
	// Read the initial revision before sharing the Engine.
	seqno, err := e.head.readTx.readSeqno(ctx)
	if err != nil {
		return err
	}
	e.recordWatchedHeadLocked(e.head.root.GetRef(), seqno, nil)
	return nil
}

// recordWatchedHeadLocked hands off one immutable transition in constant time.
// The caller holds publication authority; no feed consumer takes that lock.
func (e *Engine) recordWatchedHeadLocked(ref *bucket.ObjectRef, seqno uint64, changes *changeSummary) {
	// Replace exactly one slot, retaining only identity and detached evidence.
	notice := &watchedHead{
		before: ref.GetRootRef(),
		seqno:  seqno, publication: e.published + 1, ref: ref, changes: changes,
	}
	if head := e.pub.Load(); head != nil {
		notice.before = head.rootRef()
	}
	slot := &e.watchedHeads[notice.publication%changeWatchHeads]
	if old := slot.Load(); old != nil {
		e.watchedFloor.Store(max(e.watchedFloor.Load(), old.seqno+1))
	}
	slot.Store(notice)
	e.latestNotice.Store(notice)
}

// registerPendingChange hands off detached evidence before admission can publish.
// Overwritten evidence is harmless: the feed reports Unknown for that transition.
func (e *Engine) registerPendingChange(base, root *block.BlockRef, changes *changeSummary) *pendingChange {
	p := &pendingChange{base: base.Clone(), root: root.Clone(), changes: changes}
	e.pendingChanges[e.pendingNext.Add(1)%changeWatchHeads].Store(p)
	return p
}

// pendingSummary finds only evidence for an exact predecessor/result pair.
// The feed calls it without publication authority or the writer.
func (e *Engine) pendingSummary(before, after *block.BlockRef) *changeSummary {
	for i := range e.pendingChanges {
		p := e.pendingChanges[i].Load()
		if p != nil {
			if summary := p.summary(before, after); summary != nil {
				return summary
			}
		}
	}
	return nil
}

// watchSpanPlanLocked proves each publication and predecessor before collecting
// its detached summary. Ring overwrites or missing evidence produce Unknown.
// The caller holds the feed lock, independently of the publisher.
func (e *Engine) watchSpanPlanLocked(first uint64, firstRef *bucket.ObjectRef, last uint64) watchSpanPlan {
	// Reject spans that cannot fit in the bounded notice ring.
	if last <= first || last-first >= changeWatchHeads {
		return watchSpanPlan{unknown: true}
	}

	// Match immutable notice identities and exact predecessor roots in order.
	plan := watchSpanPlan{}
	prev := firstRef.GetRootRef()
	for pub := first + 1; pub <= last; pub++ {
		h := e.watchedHeads[pub%changeWatchHeads].Load()
		if h == nil || h.publication != pub || !h.before.EqualsRef(prev) {
			return watchSpanPlan{unknown: true}
		}
		summary := h.changes
		if summary == nil {
			summary = e.pendingSummary(h.before, h.ref.GetRootRef())
		}
		if summary == nil {
			return watchSpanPlan{unknown: true}
		}
		plan.recorded = append(plan.recorded, summary)
		prev = h.ref.GetRootRef()
	}
	return plan
}

// add merges the changes up to the head at seqno and wakes the subscriber.
// Merging is a union, so spans may arrive in any order.
func (s *changeSubscriber) add(seqno uint64, changes *world.ChangeSet) {
	// Merge under the subscriber's lock, then wake it.
	s.mtx.Lock()
	if s.idle {
		s.mtx.Unlock()
		return
	}
	s.seqno = max(s.seqno, seqno)
	s.changes = world.MergeChangeSets(s.changes, changes, changeDiffLimit)
	s.mtx.Unlock()
	s.signal()
}

// coveredThrough returns the seqno through which a caller that reported every
// change it took, the last after afterSeqno, has seen every change naming the
// subscriber: the last span w handed out, unless changes wait untaken. The
// caller must have caught up, so every later span reaches the subscriber
// through w.
func (s *changeSubscriber) coveredThrough(w *engineChangeWatch, afterSeqno uint64) uint64 {
	// Load handed first: every span through it was added before the store.
	handed := w.handedSeqno()
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.changes != nil {
		return afterSeqno
	}
	return max(afterSeqno, handed)
}

// take returns and clears the merged changes and the seqno they reach.
func (s *changeSubscriber) take() (uint64, *world.ChangeSet) {
	// Hand off the pending changes and leave none.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	changes := s.changes
	s.changes = nil
	return s.seqno, changes
}

// changeWatchErr returns why change subscribers must stop, or nil.
func (e *Engine) changeWatchErr() error {
	locked := e.changeBcast.Lock()
	defer locked.Unlock()
	if e.feedClosed {
		return e.closedErr()
	}
	return nil
}

// signal wakes the subscriber without blocking.
func (s *changeSubscriber) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// _ is a type assertion
var _ world.IndexedChangeWatcher = (*Engine)(nil)
