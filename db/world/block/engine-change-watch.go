package world_block

import (
	"context"
	"maps"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/aperturerobotics/util/broadcast"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
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
// The count survives idle watches and is guarded by the feed lock.
func (e *Engine) GetUnknownChangeCount() uint64 {
	locked := e.changeBcast.Lock()
	defer locked.Unlock()
	return e.unknownChanges
}

// engineChangeWatch owns notification history, subscriber membership and object
// revision wakes under a feed broadcast independent of publication authority.
// It consumes immutable notices and detached summaries. It holds only its base
// and target while resolving, then delivers through a separate subscriber index.
// Missing coverage invalidates readers conservatively. The first subscriber
// starts the worker; the last subscriber leaving or Close stops and joins it.
// Methods named Locked require the feed broadcast, never the Engine bcast.
type engineChangeWatch struct {
	// bcast is the feed's lock and broadcast, independent of publication.
	bcast *broadcast.Broadcast
	// ctx cancels a comparison in flight when the Engine closes.
	ctx context.Context
	// cancel cancels ctx.
	cancel context.CancelFunc
	// base is the last compared head. Watchers read heads at or after it.
	base *bucket_lookup.Cursor
	// baseSeqno is the seqno of base.
	baseSeqno uint64
	// basePublication identifies the base in the local publication order.
	basePublication uint64
	// baseRelease releases the retention hold on base.
	baseRelease func()
	// waiters maps each watched object key to its wake channel.
	waiters map[string]*objectKeyWake
	// subs are the change subscribers, for the idle check and the wakes that
	// reach every subscriber.
	subs map[*changeSubscriber]struct{}
	// indexMtx guards index. It is never held together with bcast: indexing
	// a subscriber lists every read its interest holds, and World writes
	// and reads take bcast, so they must never wait behind it.
	indexMtx sync.Mutex
	// index holds the subscribers by the names in their interests, so a
	// ChangeSet tests only the subscribers it selects.
	index *world.ChangeIndex[*changeSubscriber]
	// history holds the ChangeSets of the last compared spans, oldest first,
	// each starting at the seqno the one before it reached and the last
	// reaching baseSeqno. A comparison without a ChangeSet empties it.
	history []changeSpan
	// idle closes when the last watcher leaves so the watch releases its base
	// without waiting for another head.
	idle chan struct{}
	// handed is the seqno of the last span handed to the subscribers,
	// stored after every subscriber the span selects received the span. A
	// subscriber holding no changes has seen every change through that
	// seqno that selects the subscriber.
	handed atomic.Uint64
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

// objectKeyWake is the wake shared by the waiters of one object key.
type objectKeyWake struct {
	// state is the key's last wake. It is swapped under bcast and loaded
	// without it.
	state atomic.Pointer[objectKeyState]
	// refs counts the waiters registered on the key. Guarded by bcast.
	refs int
}

// objectKeyState is one wake of an object key's waiters: the channel the
// next wake closes and, when the change watch read it, the object's revision
// at the head it compared, which every waiter of the key shares instead of
// reading the object itself.
type objectKeyState struct {
	// ch closes when the object may have changed again.
	ch chan struct{}
	// known reports that rev and found hold the object at a head.
	known bool
	// head is the Engine's published count at the head the watch read the
	// object at.
	head uint64
	// rev is the object's revision at that head.
	rev uint64
	// found reports that the object exists at that head.
	found bool
}

// WaitObjectRev waits until the object at key reaches rev and returns its
// revision. The change watch wakes the waiter only when a published head
// changes the object's record, so unrelated commits cost the waiter nothing.
func (e *Engine) WaitObjectRev(ctx context.Context, key string, rev uint64, ignoreNotFound bool) (uint64, error) {
	// Register before the first read so the watch compares from a head at or
	// before every head this waiter reads.
	locked := e.changeBcast.Lock()
	w, wake, err := e.watchObjectKeyLocked(key)
	locked.Unlock()
	if err != nil {
		return 0, err
	}
	defer e.unwatchObjectKey(w, key, wake)

	// Read the object, then after each wake take its revision from the wake
	// when the watch read it at the head this waiter last read or a later
	// one, and otherwise read it again.
	var read uint64
	st, self := wake.state.Load(), true
	for {
		var currRev uint64
		var found bool
		if self {
			// Capture the wake together with the head it guards.
			locked := e.bcast.Lock()
			if e.closed {
				locked.Unlock()
				return 0, e.closedErr()
			}
			if err := e.initializeHeadReadTx(ctx); err != nil {
				locked.Unlock()
				return 0, err
			}
			readTx := e.head.readTx
			read = e.published
			st = wake.state.Load()
			locked.Unlock()

			// Read the captured head without holding publication authority.
			var err error
			currRev, found, err = world.GetObjectRev(ctx, readTx, key)
			if readTx.state.discarded.Load() {
				continue
			}
			if err != nil {
				return 0, err
			}
		} else {
			currRev, found, read = st.rev, st.found, st.head
		}

		// Return a satisfied revision.
		if found && currRev >= rev {
			return currRev, nil
		}
		if !found && !ignoreNotFound {
			return 0, world.ErrObjectNotFound
		}
		// Sleep until a head changes this object or the watch loses track.
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-st.ch:
		}
		st = wake.state.Load()
		self = !st.known || st.head < read
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
	return w, w.addWaiterLocked(key), nil
}

// unwatchObjectKey removes a waiter registration from its change watch.
func (e *Engine) unwatchObjectKey(w *engineChangeWatch, key string, wake *objectKeyWake) {
	// Drop the waiter, and the key once no waiter shares it.
	locked := e.changeBcast.Lock()
	w.removeWaiterLocked(key, wake)
	locked.Unlock()
}

// startChangeWatchLocked returns the running change watch, starting it from
// the published head when none runs. The caller must hold bcast.
func (e *Engine) startChangeWatchLocked() (*engineChangeWatch, error) {
	// Refuse new watchers once Close has begun draining.
	if e.feedClosed {
		return nil, e.closedErr()
	}
	if w := e.changeWatch; w != nil {
		return w, nil
	}

	// Start from the published head, held while it is the base.
	base, seqno, publication, release, _ := e.captureWatchHead()
	if base == nil {
		return nil, e.closedErr()
	}
	w := newEngineChangeWatch(&e.changeBcast, base, seqno, publication, release)
	e.changeWatch = w
	e.changeWatches++
	go e.runChangeWatch(w)
	return w, nil
}

// runChangeWatch resolves each span of published heads, wakes changed object
// waiters and hands proven summaries or Unknown to subscribers. It exits when
// no watcher remains or the Engine closes.
func (e *Engine) runChangeWatch(w *engineChangeWatch) {
	for {
		// End the feed before Close releases the stores its held heads depend on.
		locked := e.changeBcast.Lock()
		if e.feedClosed || w.emptyLocked() {
			e.stopChangeWatchLocked(w)
			locked.Unlock()
			w.releaseBase()
			locked = e.changeBcast.Lock()
			e.changeWatches--
			locked.Broadcast()
			locked.Unlock()
			return
		}

		// Wait without taking another root hold when publication has not advanced.
		signal := e.headSignal.Load()
		if e.latestNotice.Load().publication == w.basePublication && !signal.closed {
			w.waitLocked(&locked, signal.changed)
			continue
		}

		// Hold the immutable current head without publication authority.
		next, nextSeqno, nextHead, nextRelease, changed := e.captureWatchHead()
		if next == nil {
			locked.Unlock()
			continue
		}
		if nextHead == w.basePublication {
			nextRelease()
			w.waitLocked(&locked, changed)
			continue
		}
		listChanges := len(w.subs) != 0
		plan := e.watchSpanPlanLocked(w.basePublication, w.baseRefLocked(), nextHead)
		locked.Unlock()

		// Missing coverage wakes readers conservatively without fetching tree pages.
		changes := resolveWatchSpan(plan)
		var keys []string
		if !changes.Unknown {
			keys = changes.Keys
			if keys == nil {
				keys = []string{}
			}
		}
		compareCtx, _ := block.WithoutPeerWait(block.WithReadClass(w.ctx, block.ReadUrgent))
		if changes.Unknown {
			keys, _ = e.compareWatchedObjectKeys(compareCtx, w, w.base, next)
		}
		revs := e.readWatchedRevs(compareCtx, w, next, keys)

		// Keep history, revision wakes and subscriber work on the feed's own lock.
		locked = e.changeBcast.Lock()
		if listChanges && changes.Unknown {
			e.unknownChanges++
		}
		if keys == nil {
			w.wakeAllLocked()
		}
		w.wakeKeysLocked(keys, nextHead, revs)
		prevRelease := w.advanceLocked(next, nextSeqno, nextHead, nextRelease, changes)
		locked.Unlock()
		w.handOut(nextSeqno, changes)
		prevRelease()
	}
}

// captureWatchHead takes a current-head hold without publication authority.
// Loading the wake first ensures a later publication cannot be slept through.
func (e *Engine) captureWatchHead() (*bucket_lookup.Cursor, uint64, uint64, func(), <-chan struct{}) {
	// Hold an immutable descriptor, including its exact notice identity.
	signal := e.headSignal.Load()
	head := e.acquireHead()
	if head == nil {
		return nil, 0, 0, nil, nil
	}
	base := head.cloneRoot()
	return base, head.seqno, head.publication, func() { base.Release(); head.release() }, signal.changed
}

// compareWatchedObjectKeys compares only the object keys w's waiters watch
// between two held heads. It returns an empty non-nil slice when no waiter is
// registered.
func (e *Engine) compareWatchedObjectKeys(
	ctx context.Context,
	w *engineChangeWatch,
	before, after *bucket_lookup.Cursor,
) ([]string, error) {
	// Snapshot the watched keys under bcast, returning early when none wait.
	locked := e.changeBcast.Lock()
	watched := w.watchedKeysLocked()
	locked.Unlock()
	if len(watched) == 0 {
		return []string{}, nil
	}

	// Compare only the watched keys between the two heads.
	_, beforeBcs := before.BuildTransactionWithStore(nil, e.writeBlockStore)
	_, afterBcs := after.BuildTransactionWithStore(nil, e.writeBlockStore)
	return ChangedObjectKeysIn(ctx, beforeBcs, afterBcs, watched)
}

// readWatchedRevs reads, at head, the revision of each changed key some
// waiter watches, once for all of the key's waiters. It returns nil when no
// waiter watches a changed key or the read fails. The waiters then read the
// object themselves.
func (e *Engine) readWatchedRevs(ctx context.Context, w *engineChangeWatch, head *bucket_lookup.Cursor, keys []string) map[string]objectKeyState {
	// Keep the changed keys that have waiters.
	locked := e.changeBcast.Lock()
	watched := w.watchedOfLocked(keys)
	locked.Unlock()
	if len(watched) == 0 {
		return nil
	}

	// Read their records from the head's object tree.
	_, bcs := head.BuildTransactionWithStore(nil, e.writeBlockStore)
	if root, err := UnmarshalWorld(ctx, bcs); err != nil || root == nil {
		return nil
	}
	tree, err := kvtx_block.BuildKvTransaction(ctx, bcs.FollowSubBlock(1), false)
	if err != nil {
		return nil
	}
	defer tree.Discard()
	revs := make(map[string]objectKeyState, len(watched))
	for _, key := range watched {
		obj, err := readObjectRecord(ctx, tree, key)
		if err != nil {
			return nil
		}
		revs[key] = objectKeyState{rev: obj.GetRev(), found: obj != nil}
	}
	return revs
}

// newEngineChangeWatch returns a watch under the feed broadcast whose base is
// the held head base at seqno and publication, which release lets go. The
// caller starts its loop.
func newEngineChangeWatch(bcast *broadcast.Broadcast, base *bucket_lookup.Cursor, seqno, publication uint64, release func()) *engineChangeWatch {
	ctx, cancel := context.WithCancel(context.Background())
	w := &engineChangeWatch{
		bcast:           bcast,
		ctx:             ctx,
		cancel:          cancel,
		base:            base,
		baseSeqno:       seqno,
		basePublication: publication,
		baseRelease:     release,
		waiters:         make(map[string]*objectKeyWake),
		subs:            make(map[*changeSubscriber]struct{}),
		index:           world.NewChangeIndex[*changeSubscriber](),
		idle:            make(chan struct{}),
	}
	w.handed.Store(seqno)
	return w
}

// leave removes sub from the index and then from the watch, which may let the
// watch go idle. The caller must not hold bcast.
func (w *engineChangeWatch) leave(sub *changeSubscriber) {
	// Leave the index first, under its own lock.
	w.unindexSub(sub)

	// Then leave the watch.
	locked := w.bcast.Lock()
	w.removeSubLocked(sub)
	locked.Unlock()
}

// baseRefLocked returns the reference of the base. The caller must hold
// bcast.
func (w *engineChangeWatch) baseRefLocked() *bucket.ObjectRef {
	return w.base.GetRef()
}

// waitLocked releases the bcast lock the caller holds and waits until changed
// closes or the last watcher leaves.
func (w *engineChangeWatch) waitLocked(locked *broadcast.Locked, changed <-chan struct{}) {
	idle := w.idle
	locked.Unlock()
	select {
	case <-changed:
	case <-idle:
	}
}

// advanceLocked makes the held head next the base and returns the release of
// the previous base, which the caller runs once it has handed the span out.
// It records complete and Unknown spans alike, preserving their coverage.
// The caller holds the feed broadcast.
func (w *engineChangeWatch) advanceLocked(
	next *bucket_lookup.Cursor,
	seqno, publication uint64,
	release func(),
	changes *world.ChangeSet,
) func() {
	// Record the span, evicting the oldest at capacity.
	if len(w.history) == changeWatchHeads {
		w.history = slices.Delete(w.history, 0, 1)
	}
	w.history = append(w.history, changeSpan{
		after:    w.baseSeqno,
		afterRef: w.base.GetRef(),
		seqno:    seqno,
		changes:  changes,
	})

	// Advance the base.
	prevRelease := w.baseRelease
	w.base, w.baseSeqno, w.basePublication, w.baseRelease = next, seqno, publication, release
	return prevRelease
}

// handOut hands the span reaching seqno to the subscribers a changed key
// selects, and to the other selected ones that accept it, then marks it handed.
// It runs after advanceLocked recorded the span: a subscriber indexes itself
// before it reads the base, so one missing from the index here reads a base
// at or past this span and merges it from the history or its catch-up
// instead. The caller must not hold bcast.
func (w *engineChangeWatch) handOut(seqno uint64, changes *world.ChangeSet) {
	// Find the subscribers the changes select.
	w.indexMtx.Lock()
	accepted, tested := w.index.Candidates(changes)
	w.indexMtx.Unlock()

	// Hand the changes out, testing the merely named subscribers outside
	// the locks.
	for _, sub := range accepted {
		sub.add(seqno, changes)
	}
	for _, sub := range tested {
		if sub.interest.Load().Accepts(changes) {
			sub.add(seqno, changes)
		}
	}
	w.handed.Store(seqno)
}

// handedSeqno returns the seqno of the last span handed to the subscribers.
func (w *engineChangeWatch) handedSeqno() uint64 {
	return w.handed.Load()
}

// releaseBase releases the retention hold on the base of a stopped watch.
func (w *engineChangeWatch) releaseBase() {
	w.baseRelease()
}

// addWaiterLocked registers a waiter on key and returns the wake it shares
// with the key's other waiters. The caller must hold bcast.
func (w *engineChangeWatch) addWaiterLocked(key string) *objectKeyWake {
	wake := w.waiters[key]
	if wake == nil {
		wake = &objectKeyWake{}
		wake.state.Store(&objectKeyState{ch: make(chan struct{})})
		w.waiters[key] = wake
	}
	wake.refs++
	return wake
}

// removeWaiterLocked drops a waiter registered on key with wake, and the key
// once no waiter shares it, which may let w go idle. The caller must hold
// bcast.
func (w *engineChangeWatch) removeWaiterLocked(key string, wake *objectKeyWake) {
	wake.refs--
	if wake.refs == 0 {
		delete(w.waiters, key)
	}
	w.checkIdleLocked()
}

// watchedKeysLocked returns the object keys some waiter watches. The caller
// must hold bcast.
func (w *engineChangeWatch) watchedKeysLocked() []string {
	return slices.Collect(maps.Keys(w.waiters))
}

// watchedOfLocked returns, in a new slice, the keys of keys some waiter
// watches. The caller must hold bcast.
func (w *engineChangeWatch) watchedOfLocked(keys []string) []string {
	return slices.DeleteFunc(slices.Clone(keys), func(key string) bool {
		return w.waiters[key] == nil
	})
}

// addSubLocked joins sub to w. The caller must hold bcast.
func (w *engineChangeWatch) addSubLocked(sub *changeSubscriber) {
	w.subs[sub] = struct{}{}
}

// removeSubLocked removes sub from w, which may let w go idle. The caller
// must hold bcast.
func (w *engineChangeWatch) removeSubLocked(sub *changeSubscriber) {
	delete(w.subs, sub)
	w.checkIdleLocked()
}

// indexSub indexes sub by the names in interest under indexMtx. The caller
// must not hold bcast.
func (w *engineChangeWatch) indexSub(sub *changeSubscriber, interest *world.ChangeInterest) {
	w.indexMtx.Lock()
	w.index.Add(sub, interest)
	w.indexMtx.Unlock()
}

// unindexSub removes sub from the index under indexMtx. The caller must not
// hold bcast.
func (w *engineChangeWatch) unindexSub(sub *changeSubscriber) {
	w.indexMtx.Lock()
	w.index.Remove(sub)
	w.indexMtx.Unlock()
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
// cancels its comparisons in flight, and wakes its watchers. The caller must
// hold bcast.
func (e *Engine) stopChangeWatchLocked(w *engineChangeWatch) {
	if e.changeWatch == w {
		e.changeWatch = nil
	}
	w.stopLocked()
}

// headSeqnoLocked returns the seqno of the published head. Every published
// head is recorded for change watchers, so the newest recorded head is the
// published one. The caller must hold bcast.
func (e *Engine) headSeqnoLocked() uint64 {
	return e.latestNotice.Load().seqno
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

// wakeKeysLocked wakes the waiters of the listed keys, handing them the
// revision read at the published head numbered head when revs holds it. The
// caller must hold bcast.
func (w *engineChangeWatch) wakeKeysLocked(keys []string, head uint64, revs map[string]objectKeyState) {
	for _, key := range keys {
		wake := w.waiters[key]
		if wake == nil {
			continue
		}
		next := &objectKeyState{}
		if r, ok := revs[key]; ok {
			next.known, next.head, next.rev, next.found = true, head, r.rev, r.found
		}
		wake.publish(next)
	}
}

// stopLocked cancels w's comparisons in flight and wakes every watcher. The
// caller must hold bcast.
func (w *engineChangeWatch) stopLocked() {
	w.cancel()
	w.wakeAllLocked()
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

// signal wakes the key's current waiters, which read the object themselves.
// The caller must hold bcast.
func (k *objectKeyWake) signal() {
	k.publish(&objectKeyState{})
}

// publish wakes the key's current waiters with next and arms its channel.
// The caller must hold bcast.
func (k *objectKeyWake) publish(next *objectKeyState) {
	next.ch = make(chan struct{})
	close(k.state.Swap(next).ch)
}
