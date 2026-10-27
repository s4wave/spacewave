package world_block

import (
	"context"
	"maps"
	"slices"
	"sync"

	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
)

// changeDiffLimit bounds the changed keys one head comparison reports. For a
// larger change, the watch compares only the keys its waiters watch, and
// subscribers receive unknown changes.
const changeDiffLimit = 4096

// GetUnknownChangeCount returns the number of unknown spans produced by the
// shared change watch over this Engine's lifetime, once per comparison, not per
// subscriber. It excludes subscriber catch-up and coalescing, and comparisons
// made only for object revision waiters, which do not request a ChangeSet.
// The count survives idle watches and is guarded by the publication lock.
func (e *Engine) GetUnknownChangeCount() uint64 {
	locked := e.bcast.Lock()
	defer locked.Unlock()
	return e.unknownChanges
}

// engineChangeWatch is the Engine's change producer. It compares each new head
// with the last compared head once for all watchers, wakes the object
// revision waiters of the changed keys, hands the ChangeSet to the change
// subscribers whose interests name it and accept it, and records it so a
// subscriber starting behind merges the recorded ChangeSets instead of
// comparing heads again. A changed object record never returns to an earlier
// value, because its revision grows with every change, so comparing the last
// compared head with the newest one covers every head published between them.
// The Engine starts the watch for the first watcher and it exits when the last
// watcher leaves or the Engine closes. Its fields are guarded by the Engine
// bcast lock, except index, which indexMtx guards.
type engineChangeWatch struct {
	// ctx cancels a comparison in flight when the Engine closes.
	ctx context.Context
	// cancel cancels ctx.
	cancel context.CancelFunc
	// base is the last compared head. Watchers read heads at or after it.
	base *bucket_lookup.Cursor
	// baseSeqno is the seqno of base.
	baseSeqno uint64
	// baseRelease releases the retention hold on base.
	baseRelease func()
	// waiters maps each watched object key to its wake channel.
	waiters map[string]*objectKeyWake
	// subs are the change subscribers, for the idle check and the wakes that
	// reach every subscriber.
	subs map[*changeSubscriber]struct{}
	// indexMtx guards index. It is never held together with bcast: indexing
	// a subscriber names every read its interest holds, and World writes and
	// reads take bcast, so they must never wait behind it.
	indexMtx sync.Mutex
	// index holds the subscribers by the names in their interests, so a
	// ChangeSet tests only the subscribers it names.
	index *world.ChangeIndex[*changeSubscriber]
	// history holds the ChangeSets of the last compared spans, oldest first,
	// each starting at the seqno the one before it reached and the last
	// reaching baseSeqno. A comparison without a ChangeSet empties it.
	history []changeSpan
	// idle closes when the last watcher leaves so the watch releases its base
	// without waiting for another head.
	idle chan struct{}
}

// changeSpan is one comparison of the change watch: the changes from the head
// at after to the head at seqno.
type changeSpan struct {
	// after is the seqno of the earlier head.
	after uint64
	// afterRef is the reference of the earlier head, which tells it apart
	// from another head published at the same seqno.
	afterRef *bucket.ObjectRef
	// seqno is the seqno of the later head.
	seqno uint64
	// changes lists the changes between the two heads.
	changes *world.ChangeSet
}

// objectKeyWake is the wake channel shared by the waiters of one object key.
type objectKeyWake struct {
	// ch closes when the object may have changed and is then replaced.
	ch chan struct{}
	// refs counts the waiters registered on the key.
	refs int
}

// WaitObjectRev waits until the object at key reaches rev and returns its
// revision. The change watch wakes the waiter only when a published head
// changes the object's record, so unrelated commits cost the waiter nothing.
func (e *Engine) WaitObjectRev(ctx context.Context, key string, rev uint64, ignoreNotFound bool) (uint64, error) {
	// Discover durable commits that preceded this wait.
	if err := e.refreshWaitHead(ctx); err != nil {
		return 0, err
	}

	// Register before the first read so the watch compares from a head at or
	// before every head this waiter reads.
	locked := e.bcast.Lock()
	w, wake, err := e.watchObjectKeyLocked(key)
	locked.Unlock()
	if err != nil {
		return 0, err
	}
	defer e.unwatchObjectKey(w, key, wake)

	// Reread the object after each wake until its revision satisfies the wait.
	for {
		// Capture the wake channel together with the head it guards.
		locked := e.bcast.Lock()
		if e.closed {
			locked.Unlock()
			return 0, ErrEngineClosed
		}
		if err := e.initializeHeadReadTx(ctx); err != nil {
			locked.Unlock()
			return 0, err
		}
		readTx := e.head.readTx
		watchErr := e.headWatchErr
		woken := wake.ch
		locked.Unlock()

		// Read the captured head without holding publication authority.
		currRev, found, err := world.GetObjectRev(ctx, readTx, key)
		if readTx.state.discarded.Load() {
			continue
		}
		if err != nil {
			return 0, err
		}

		// Return a satisfied revision before considering observation failure.
		if found && currRev >= rev {
			return currRev, nil
		}
		if !found && !ignoreNotFound {
			return 0, world.ErrObjectNotFound
		}
		if watchErr != nil {
			return 0, watchErr
		}

		// Sleep until a head changes this object or the watch loses track.
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-woken:
		}
	}
}

// watchObjectKeyLocked registers a waiter on key, starting the change watch
// when none runs. The caller must hold bcast.
func (e *Engine) watchObjectKeyLocked(key string) (*engineChangeWatch, *objectKeyWake, error) {
	// Start the change watch for the first watcher.
	w, err := e.startChangeWatchLocked()
	if err != nil {
		return nil, nil, err
	}

	// Share one wake channel among the waiters of the same key.
	wake := w.waiters[key]
	if wake == nil {
		wake = &objectKeyWake{ch: make(chan struct{})}
		w.waiters[key] = wake
	}
	wake.refs++
	return w, wake, nil
}

// unwatchObjectKey removes a waiter registration from its change watch.
func (e *Engine) unwatchObjectKey(w *engineChangeWatch, key string, wake *objectKeyWake) {
	// Drop the waiter, and the key once no waiter shares it.
	locked := e.bcast.Lock()
	wake.refs--
	if wake.refs == 0 {
		delete(w.waiters, key)
	}
	w.checkIdleLocked()
	locked.Unlock()
}

// startChangeWatchLocked returns the running change watch, starting it from
// the published head when none runs. The caller must hold bcast.
func (e *Engine) startChangeWatchLocked() (*engineChangeWatch, error) {
	// Refuse new watchers once Close has begun draining.
	if e.closed {
		return nil, ErrEngineClosed
	}
	if w := e.changeWatch; w != nil {
		return w, nil
	}

	// Start from the published head, held while it is the base.
	ctx, cancel := context.WithCancel(context.Background())
	base, release := e.holdHeadLocked()
	w := &engineChangeWatch{
		ctx:         ctx,
		cancel:      cancel,
		base:        base,
		baseSeqno:   e.headSeqnoLocked(),
		baseRelease: release,
		waiters:     make(map[string]*objectKeyWake),
		subs:        make(map[*changeSubscriber]struct{}),
		index:       world.NewChangeIndex[*changeSubscriber](),
		idle:        make(chan struct{}),
	}
	e.changeWatch = w
	e.changeWatches++
	go e.runChangeWatch(w)
	return w, nil
}

// runChangeWatch compares each newly published head with the last compared
// head, wakes the waiters of the changed objects and hands the changes to the
// subscribers. It exits once no watcher remains or the Engine closes.
func (e *Engine) runChangeWatch(w *engineChangeWatch) {
	for {
		// Stop when nobody watches or the Engine closes.
		locked := e.bcast.Lock()
		if e.closed || w.emptyLocked() {
			e.stopChangeWatchLocked(w)
			locked.Unlock()
			w.cancel()
			w.baseRelease()

			// Let Close release the stores only after the base is released.
			locked = e.bcast.Lock()
			e.changeWatches--
			locked.Broadcast()
			locked.Unlock()
			return
		}

		// Wait for a head the watch has not compared. A wakeup without a new
		// head reports an observation failure, which watchers must see.
		if e.head.root.GetRef().EqualsRef(w.base.GetRef()) {
			wait, idle := e.headChanged, w.idle
			locked.Unlock()
			select {
			case <-wait:
			case <-idle:
			}

			locked = e.bcast.Lock()
			if !e.closed && e.headWatchErr != nil {
				w.wakeAllLocked()
			}
			locked.Unlock()
			continue
		}

		// Hold the new head while it is compared and while it is the base.
		next, nextRelease := e.holdHeadLocked()
		nextSeqno := e.headSeqnoLocked()
		listChanges := len(w.subs) != 0
		locked.Unlock()

		// Compare the World trees outside the publication lock.
		keys, changes, err := e.compareChangeWatchHeads(w.ctx, w, w.base, next, listChanges)
		if err != nil && w.ctx.Err() == nil {
			e.le.WithError(err).Warn("world change comparison failed; waking every watcher")
		}

		// Wake the changed objects' waiters, or all of them when the
		// comparison could not list the changes, record the span, and advance
		// the base.
		locked = e.bcast.Lock()
		if listChanges && changes.Unknown {
			e.unknownChanges++
		}
		if keys == nil {
			w.wakeAllLocked()
		}
		w.wakeKeysLocked(keys)
		w.recordLocked(changeSpan{
			after:    w.baseSeqno,
			afterRef: w.base.GetRef().CloneVT(),
			seqno:    nextSeqno,
			changes:  changes,
		}, listChanges && err == nil)
		prevRelease := w.baseRelease
		w.base, w.baseSeqno, w.baseRelease = next, nextSeqno, nextRelease
		locked.Unlock()

		// Find the subscribers the changes name after the record. A
		// subscriber indexes itself before it reads the base, so one missing
		// from the index here reads a base at or past this span and merges it
		// from the history or its catch-up instead.
		w.indexMtx.Lock()
		candidates := w.index.Candidates(changes)
		w.indexMtx.Unlock()

		// Hand the changes to the named subscribers that accept them, testing
		// each outside the locks.
		for _, sub := range candidates {
			if sub.interest.Accepts(changes) {
				sub.add(nextSeqno, changes)
			}
		}
		prevRelease()
	}
}

// compareChangeWatchHeads compares two held heads. keys lists the changed
// object records, including revisions that only follow changed quads; for a
// change too large to list, it lists only the changed keys w's waiters watch.
// keys is nil when the comparison failed. When listChanges is set and the
// change could be listed, changes is the ChangeSet for subscribers; otherwise
// it is unknown.
func (e *Engine) compareChangeWatchHeads(
	ctx context.Context,
	w *engineChangeWatch,
	before, after *bucket_lookup.Cursor,
	listChanges bool,
) (keys []string, changes *world.ChangeSet, err error) {
	// List the changed object records for the key waiters.
	_, beforeBcs := before.BuildTransactionWithStore(nil, e.writeBlockStore)
	_, afterBcs := after.BuildTransactionWithStore(nil, e.writeBlockStore)
	keys, complete, err := ChangedObjectKeys(ctx, beforeBcs, afterBcs, changeDiffLimit)
	if err != nil {
		return nil, world.NewUnknownChangeSet(), err
	}

	// Compare only the watched keys of a change too large to list. A waiter
	// registered after after was published reads a head at or past it.
	if !complete {
		locked := e.bcast.Lock()
		watched := slices.Collect(maps.Keys(w.waiters))
		locked.Unlock()
		keys, err = ChangedObjectKeysIn(ctx, beforeBcs, afterBcs, watched)
		return keys, world.NewUnknownChangeSet(), err
	}

	// Add the changed quads for the subscribers.
	changes = world.NewUnknownChangeSet()
	if listChanges {
		changes, err = worldChangesWithKeys(ctx, beforeBcs, afterBcs, slices.Clone(keys), changeDiffLimit)
	}
	return keys, changes, err
}

// recordLocked records the span the watch compared when it listed the
// changes, and otherwise forgets every span, since a span without its
// ChangeSet breaks the chain from the earlier ones to the base. The caller
// must hold bcast.
func (w *engineChangeWatch) recordLocked(span changeSpan, listed bool) {
	if !listed {
		w.history = nil
		return
	}
	if len(w.history) == changeWatchHeads {
		w.history = slices.Delete(w.history, 0, 1)
	}
	w.history = append(w.history, span)
}

// spansSinceLocked returns a copy of the recorded spans from the head at after
// to the base. It returns false when no recorded span starts at that head. The
// caller must hold bcast.
func (w *engineChangeWatch) spansSinceLocked(after uint64, afterRef *bucket.ObjectRef) ([]changeSpan, bool) {
	i := slices.IndexFunc(w.history, func(span changeSpan) bool {
		return span.after == after && span.afterRef.EqualsRef(afterRef)
	})
	if i < 0 {
		return nil, false
	}
	return slices.Clone(w.history[i:]), true
}

// mergeSpans merges the ChangeSets of consecutive spans.
func mergeSpans(spans []changeSpan) *world.ChangeSet {
	var changes *world.ChangeSet
	for _, span := range spans {
		changes = world.MergeChangeSets(changes, span.changes, changeDiffLimit)
	}
	return changes
}

// stopChangeWatchLocked detaches w so the next watcher starts a fresh watch,
// and wakes its watchers. The caller must hold bcast.
func (e *Engine) stopChangeWatchLocked(w *engineChangeWatch) {
	if e.changeWatch == w {
		e.changeWatch = nil
	}
	w.wakeAllLocked()
}

// headSeqnoLocked returns the seqno of the published head. Every published
// head is recorded for change watchers, so the newest recorded head is the
// published one. The caller must hold bcast.
func (e *Engine) headSeqnoLocked() uint64 {
	return e.watchedHeads[len(e.watchedHeads)-1].seqno
}

// emptyLocked reports whether no watcher remains. The caller must hold bcast.
func (w *engineChangeWatch) emptyLocked() bool {
	return len(w.waiters) == 0 && len(w.subs) == 0
}

// checkIdleLocked lets an idle watch release its base without waiting for
// another head. The caller must hold bcast.
func (w *engineChangeWatch) checkIdleLocked() {
	if w.emptyLocked() {
		close(w.idle)
		w.idle = make(chan struct{})
	}
}

// wakeKeysLocked wakes the waiters of the listed keys. The caller must hold bcast.
func (w *engineChangeWatch) wakeKeysLocked(keys []string) {
	for _, key := range keys {
		if wake := w.waiters[key]; wake != nil {
			wake.signal()
		}
	}
}

// wakeAllLocked wakes every watcher. The caller must hold bcast.
func (w *engineChangeWatch) wakeAllLocked() {
	for _, wake := range w.waiters {
		wake.signal()
	}
	for sub := range w.subs {
		sub.signal()
	}
}

// signal wakes the key's current waiters and arms a fresh channel.
func (k *objectKeyWake) signal() {
	close(k.ch)
	k.ch = make(chan struct{})
}
