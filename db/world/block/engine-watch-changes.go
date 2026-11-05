package world_block

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"

	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
)

// changeWatchHeads bounds the published heads the Engine records so a new
// change subscriber can compare the head it last saw against a later one.
const changeWatchHeads = 128

// watchedHead is a published head recorded for change watchers.
type watchedHead struct {
	// seqno is the World seqno of the head.
	seqno uint64
	// root is the root cursor of the head.
	root *bucket_lookup.Cursor
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
// that far back, it compares the head at afterSeqno against the watch's base
// and continues from there, sharing the comparison with the other subscribers
// that start at the same head. The Engine records the last changeWatchHeads
// heads it published, starting with the head it opened at, so a recent
// afterSeqno is precise. Recording does not pin a head, so superseded
// Worlds stay collectible, and a base the volume collected reports unknown
// changes. The ChangeSet is also unknown when the head at afterSeqno is no
// longer recorded, when two recorded heads share that seqno, or when the
// change is too large to list.
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
	defer done()

	// Copy the recorded spans when the change watch starts past afterSeqno and
	// recorded every span since the head at afterSeqno. A reconnect wave
	// subscribes thousands of watchers at once, so the merge runs unlocked.
	locked := e.bcast.Lock()
	caughtUp := afterSeqno >= w.baseSeqno
	var spans []changeSpan
	if base := e.watchedHeadLocked(afterSeqno); !caughtUp && base != nil {
		spans, caughtUp = w.spansSinceLocked(afterSeqno, base.GetRef())
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
// interest, and returns done, which ends the call. A subscriber keeps the
// watch running until it leaves.
//
// With a WatchSlot in ctx, it reuses the subscriber the slot kept from an
// earlier call on the same watch, emptied of what it merged, and moves its
// index entry by the difference between the interests, so a view that reruns
// after each change touches only the names its reads gained or lost; done
// puts the subscriber back in the slot, which leaves the watch when it
// closes. The reuse keeps a fresh subscriber's order, indexing before the
// caller reads the watch's base, so the caller's catch-up from afterSeqno
// covers every change the emptied subscriber dropped. Without a slot, done
// leaves the watch.
func (e *Engine) subscribeChanges(ctx context.Context, interest *world.ChangeInterest) (*engineChangeWatch, *changeSubscriber, func(), error) {
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
			w.subs[sub] = struct{}{}
		}
	}
	locked.Unlock()

	// Release an obsolete subscription before completing this claim.
	if stale != nil {
		stale.ReleaseWatch()
	}
	if err != nil {
		if slot != nil {
			slot.Put(nil)
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
	w.indexMtx.Lock()
	w.index.Add(sub, interest)
	w.indexMtx.Unlock()

	// End the call by keeping the subscriber in the slot, idle, or by leaving.
	if slot == nil {
		return w, sub, func() { e.leaveChangeWatch(w, sub) }, nil
	}
	return w, sub, func() {
		sub.mtx.Lock()
		sub.idle, sub.changes = true, nil
		sub.mtx.Unlock()
		slot.Put(&slotSubscriber{e: e, w: w, sub: sub})
	}, nil
}

// leaveChangeWatch removes sub from w's index and then from w, which may let
// w go idle.
func (e *Engine) leaveChangeWatch(w *engineChangeWatch, sub *changeSubscriber) {
	// Leave the index first, under its own lock.
	w.indexMtx.Lock()
	w.index.Remove(sub)
	w.indexMtx.Unlock()

	// Then leave the watch.
	locked := e.bcast.Lock()
	delete(w.subs, sub)
	w.checkIdleLocked()
	locked.Unlock()
}

// watchedHeadLocked returns the one recorded root at seqno, or nil when the
// seqno is below the floor, unrecorded, or shared by different roots. The
// caller must hold bcast.
func (e *Engine) watchedHeadLocked(seqno uint64) *bucket_lookup.Cursor {
	if seqno < e.watchedFloor {
		return nil
	}
	var found *bucket_lookup.Cursor
	for _, h := range e.watchedHeads {
		if h.seqno != seqno {
			continue
		}
		if found != nil && !found.GetRef().EqualsRef(h.root.GetRef()) {
			return nil
		}
		found = h.root
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
	e.recordWatchedHeadLocked(e.head.root, seqno)
	return nil
}

// recordWatchedHeadLocked records a newly published head for change watchers.
// It returns the evicted oldest head for release after the lock. Eviction
// raises the floor above the evicted seqno, so no watcher compares against
// another head that shared it. The caller must hold bcast.
func (e *Engine) recordWatchedHeadLocked(root *bucket_lookup.Cursor, seqno uint64) *watchedHead {
	// Evict the oldest head at capacity.
	var evicted *watchedHead
	if len(e.watchedHeads) == changeWatchHeads {
		evicted = e.watchedHeads[0]
		e.watchedFloor = max(e.watchedFloor, evicted.seqno+1)
		e.watchedHeads = slices.Delete(e.watchedHeads, 0, 1)
	}
	e.watchedHeads = append(e.watchedHeads, &watchedHead{seqno: seqno, root: root.Clone()})
	return evicted
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
		return ErrEngineClosed
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
