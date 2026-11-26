package world_block

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
)

// changeWatchHeads bounds the published heads the Engine records so a new
// change subscriber can catch up from the head it last saw against a later one.
const changeWatchHeads = 128

// maxWatchComparisons bounds the per-span tree comparisons for publications
// without a recorded ChangeSet. Beyond it, an unknown change lets subscribers
// reread rather than making them wait for an arbitrarily long history walk.
const maxWatchComparisons = 8

// watchedHead is a published head recorded for change watchers. It holds only
// the head's ref and ChangeSet, never a cursor, so past Worlds stay collectible.
type watchedHead struct {
	// seqno is the World seqno of the head.
	seqno uint64
	// publication is the local publication number, independent of the seqno.
	publication uint64
	// ref is the ObjectRef of the head.
	ref *bucket.ObjectRef
	// changes is the ChangeSet from the preceding publication to this head,
	// or nil when the head arrived without one.
	changes *world.ChangeSet
}

// pendingChange pairs an in-flight local commit's base and candidate roots with
// the ChangeSet it accumulated, so followHead or a durable-base install that
// publishes the candidate ahead of CommitBlockTransaction still records it.
type pendingChange struct {
	// base is the root the local commit built on.
	base *block.BlockRef
	// root is the candidate root the local commit produced.
	root *block.BlockRef
	// changes is the ChangeSet accumulated from base to root.
	changes *world.ChangeSet
}

// watchedRefPair is one adjacent publication pair whose ChangeSet was not
// recorded and must be derived by comparing the two trees.
type watchedRefPair struct {
	// before is the ObjectRef of the earlier publication.
	before *bucket.ObjectRef
	// after is the ObjectRef of the later publication.
	after *bucket.ObjectRef
}

// watchSpanPlan is the work collected under bcast to compute the ChangeSet
// across a span of publications: recorded ChangeSets to merge and unrecorded
// adjacent head pairs to compare.
type watchSpanPlan struct {
	// recorded holds the ChangeSets recorded in the ring for the span.
	recorded []*world.ChangeSet
	// pairs holds the adjacent pairs without a recorded ChangeSet.
	pairs []watchedRefPair
	// unknown reports that the span cannot be certified: history is missing,
	// a recorded ChangeSet is unknown, or more than maxWatchComparisons pairs
	// require tree comparison.
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
	// idle is set while a WatchSlot keeps the subscriber between calls; it
	// merges no changes then, since the next call catches up from its own
	// afterSeqno.
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
	// e is the Engine whose watch sub joined.
	e *Engine
	// w is the change watch sub joined.
	w *engineChangeWatch
	// sub is the kept subscriber.
	sub *changeSubscriber
}

// ReleaseWatch leaves the change watch.
func (s *slotSubscriber) ReleaseWatch() {
	s.e.leaveChangeWatch(s.w, s.sub)
}

// WatchChanges calls cb with each later seqno whose changes touches accepts,
// and the accepted changes since the previous call. The changes come from the
// Engine's change watch, which compares each new head with the last compared
// one once for every watcher, so they need no changelog, and runs touches on
// each ChangeSet so a change the subscriber does not watch never wakes it.
// Calls coalesce while cb runs. cb must not modify the ChangeSet, which other
// subscribers share.
//
// A subscriber that starts behind the change watch merges the ChangeSets the
// watch recorded since the head at afterSeqno. When the watch did not record
// that far back, catch-up merges the ChangeSets each local commit recorded in
// the Engine's ring of changeWatchHeads publications, falling back to a shared
// tree comparison for any publication that arrived without one while its pages
// remain readable. Recording keeps only each publication's ref and ChangeSet
// and holds no cursor or ReadRoots entry, so superseded Worlds stay
// collectible and trim stores no past heads for the watch. The ChangeSet is
// unknown when the head at afterSeqno is no longer recorded, when two recorded
// heads share that seqno, when an unrecorded publication's pages were
// collected, or when the change is too large to list.
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
// naming none of them never tests or wakes it.
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
	locked := e.bcast.Lock()
	caughtUp := afterSeqno >= w.baseSeqno
	var spans []changeSpan
	if base := e.watchedHeadLocked(afterSeqno); !caughtUp && base != nil {
		spans, caughtUp = w.spansSinceLocked(afterSeqno, base.ref)
	}
	baseSeqno := w.baseSeqno
	locked.Unlock()

	// Merge the recorded spans, or the shared comparison from the head at
	// afterSeqno. The first wake checks whether the watch already ended.
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
// interest, and returns done, which ends the call with the seqno it covered. A subscriber keeps the
// watch running until it leaves.
//
// With a WatchSlot in ctx, it reuses the subscriber the slot kept from an
// earlier call on the same watch, emptied of what it merged, and moves its
// index entry by the difference between the interests, so a view that reruns
// after each change touches only the names its reads gained or lost; done
// puts the subscriber back in the slot with the seqno the call covered, and
// the slot leaves the watch when it closes. The reuse keeps a fresh subscriber's order, indexing before the
// caller reads the watch's base, so the caller's catch-up from afterSeqno
// covers every change the emptied subscriber dropped. Without a slot, done
// leaves the watch.
func (e *Engine) subscribeChanges(ctx context.Context, interest *world.ChangeInterest) (*engineChangeWatch, *changeSubscriber, func(covered uint64), error) {
	// Claim the slot and the subscriber it kept on this Engine.
	slot := world.WatchSlotFromContext(ctx)
	var kept *slotSubscriber
	if slot != nil {
		held, ok := slot.Take()
		if !ok {
			slot = nil
		} else if held != nil {
			if kept, _ = held.(*slotSubscriber); kept == nil || kept.e != e {
				held.ReleaseWatch()
				kept = nil
			}
		}
	}

	// Join the running watch, keeping the subscriber when it joined the same
	// one; a stopped watch took its subscribers with it.
	locked := e.bcast.Lock()
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
		return w, sub, func(uint64) { e.leaveChangeWatch(w, sub) }, nil
	}
	return w, sub, func(covered uint64) {
		sub.mtx.Lock()
		sub.idle, sub.changes = true, nil
		sub.mtx.Unlock()
		slot.Put(&slotSubscriber{e: e, w: w, sub: sub}, covered)
	}, nil
}

// leaveChangeWatch removes sub from w's index and then from w, which may let
// w go idle.
func (e *Engine) leaveChangeWatch(w *engineChangeWatch, sub *changeSubscriber) {
	// Leave the index first, under its own lock.
	w.unindexSub(sub)

	// Then leave the watch.
	locked := e.bcast.Lock()
	w.removeSubLocked(sub)
	locked.Unlock()
}

// watchedHeadLocked returns the one recorded head at seqno, or nil when the
// seqno is below the floor, unrecorded, or shared by different refs. The
// caller must hold bcast.
func (e *Engine) watchedHeadLocked(seqno uint64) *watchedHead {
	if seqno < e.watchedFloor {
		return nil
	}
	var found *watchedHead
	for _, h := range e.watchedHeads {
		if h.seqno != seqno {
			continue
		}
		if found != nil && !found.ref.EqualsRef(h.ref) {
			return nil
		}
		found = h
	}
	return found
}

// recordInitialHead records the head the Engine opens at. NewEngine calls it
// before the Engine is shared.
func (e *Engine) recordInitialHead(ctx context.Context) error {
	seqno, err := e.head.readTx.state.GetSeqno(ctx)
	if err != nil {
		return err
	}
	e.recordWatchedHeadLocked(e.head.root.GetRef(), seqno, nil)
	return nil
}

// recordWatchedHeadLocked records a newly published head for change watchers.
// Eviction raises the floor above the evicted seqno, so no watcher compares
// against another head that shared it. The caller must hold bcast.
func (e *Engine) recordWatchedHeadLocked(ref *bucket.ObjectRef, seqno uint64, changes *world.ChangeSet) {
	// Evict the oldest head at capacity.
	if len(e.watchedHeads) == changeWatchHeads {
		evicted := e.watchedHeads[0]
		e.watchedFloor = max(e.watchedFloor, evicted.seqno+1)
		e.watchedHeads = slices.Delete(e.watchedHeads, 0, 1)
	}
	// signalHeadLocked assigns this publication number before releasing bcast.
	e.watchedHeads = append(e.watchedHeads, &watchedHead{
		seqno:       seqno,
		publication: e.published + 1,
		ref:         ref.CloneVT(),
		changes:     changes,
	})
}

// registerPendingChange records a local commit's ChangeSet while its candidate
// is in flight and returns a cleanup function that removes it once the commit
// finishes or fails.
func (e *Engine) registerPendingChange(base, root *block.BlockRef, changes *world.ChangeSet) func() {
	// Append the pending entry under bcast so followHead sees it immediately.
	p := &pendingChange{
		base:    base.Clone(),
		root:    root.Clone(),
		changes: changes,
	}
	locked := e.bcast.Lock()
	e.pendingChanges = append(e.pendingChanges, p)
	locked.Unlock()

	// Return an idempotent cleanup func for commit exit.
	return func() {
		locked := e.bcast.Lock()
		e.pendingChanges = slices.DeleteFunc(e.pendingChanges, func(o *pendingChange) bool {
			return o == p
		})
		locked.Unlock()
	}
}

// takePendingChangeLocked returns and removes the ChangeSet of an in-flight
// local commit whose base and candidate match the transition from base to
// root, or nil when none matches. The caller must hold bcast.
func (e *Engine) takePendingChangeLocked(base, root *block.BlockRef) *world.ChangeSet {
	// Match an in-flight commit whose exact base and candidate roots agree.
	i := slices.IndexFunc(e.pendingChanges, func(p *pendingChange) bool {
		return p.base.EqualsRef(base) && p.root.EqualsRef(root)
	})
	if i < 0 {
		return nil
	}

	// Remove and return the matched ChangeSet.
	changes := e.pendingChanges[i].changes
	e.pendingChanges = slices.Delete(e.pendingChanges, i, i+1)
	return changes
}

// watchSpanPlanLocked collects the recorded ChangeSets and unrecorded adjacent
// ref pairs for publications strictly after first through last. firstRef is the
// head ref at publication first. A gap in retained history, an unknown recorded
// ChangeSet, or more than maxWatchComparisons unrecorded pairs yields an
// unknown plan. The caller must hold bcast.
func (e *Engine) watchSpanPlanLocked(first uint64, firstRef *bucket.ObjectRef, last uint64) watchSpanPlan {
	// Require every publication in (first, last] to be retained in order.
	if last <= first || len(e.watchedHeads) == 0 {
		return watchSpanPlan{unknown: true}
	}
	oldest := e.watchedHeads[0].publication
	if first+1 < oldest || last-oldest >= uint64(len(e.watchedHeads)) {
		return watchSpanPlan{unknown: true}
	}

	// Collect recorded ChangeSets and adjacent pairs that need tree diffing.
	var plan watchSpanPlan
	prevRef := firstRef
	for pub := first + 1; pub <= last; pub++ {
		head := e.watchedHeads[pub-oldest]
		if head.changes != nil {
			if head.changes.Unknown {
				return watchSpanPlan{unknown: true}
			}
			plan.recorded = append(plan.recorded, head.changes)
			prevRef = head.ref
			continue
		}
		if len(plan.pairs) >= maxWatchComparisons {
			return watchSpanPlan{unknown: true}
		}
		plan.pairs = append(plan.pairs, watchedRefPair{before: prevRef, after: head.ref})
		prevRef = head.ref
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
	locked := e.bcast.Lock()
	defer locked.Unlock()
	if e.closed {
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
