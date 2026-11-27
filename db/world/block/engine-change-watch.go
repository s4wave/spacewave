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
// comparing heads again. A complete change set covers every publication in
// its span: a reader may have seen an object at an intermediate head even
// if it is absent at both ends. Missing history is an unknown change.
// The Engine starts the watch for the first watcher and it exits when the last
// watcher leaves or the Engine closes. Its fields are guarded by the Engine
// bcast lock, except index, which indexMtx guards.
//
// Callers work on the watch only through its methods. A running watch takes
// watchers (addWaiterLocked, addSubLocked, indexSub) and loses them
// (removeWaiterLocked, removeSubLocked, unindexSub), and the last to leave
// closes its idle channel. For each new head it advances its base
// (advanceLocked) and hands the span out (handOut), and between heads it
// waits (waitLocked). Once stopped (stopLocked) it cancels its comparisons
// and wakes every watcher, and its loop releases the base (releaseBase). The
// methods named Locked need bcast. The ones that take indexMtx must be called
// without bcast.
type engineChangeWatch struct {
	// bcast is the Engine's publication lock and broadcast.
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
	// catchUps holds the comparisons from recorded heads to bases of the
	// watch, oldest first, which the subscribers that start behind the watch
	// at the same head share.
	catchUps []*changeCatchUp
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
	locked := e.bcast.Lock()
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
	locked := e.bcast.Lock()
	w.removeWaiterLocked(key, wake)
	locked.Unlock()
}

// startChangeWatchLocked returns the running change watch, starting it from
// the published head when none runs. The caller must hold bcast.
func (e *Engine) startChangeWatchLocked() (*engineChangeWatch, error) {
	// Refuse new watchers once Close has begun draining.
	if e.closed {
		return nil, e.closedErr()
	}
	if w := e.changeWatch; w != nil {
		return w, nil
	}

	// Start from the published head, held while it is the base.
	base, release := e.holdHeadLocked()
	w := newEngineChangeWatch(&e.bcast, base, e.headSeqnoLocked(), e.published, release)
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
			w.releaseBase()

			// Let Close release the stores only after the base is released.
			locked = e.bcast.Lock()
			e.changeWatches--
			locked.Broadcast()
			locked.Unlock()
			return
		}

		// Wait for a head the watch has not compared. A wakeup without a new
		// head reports an observation failure, which watchers must see.
		if e.head.root.GetRef().EqualsRef(w.baseRefLocked()) {
			w.waitLocked(&locked, e.headSignal.Load().changed)
			continue
		}

		// Hold the new head while it is compared and while it is the base.
		next, nextRelease := e.holdHeadLocked()
		nextSeqno := e.headSeqnoLocked()
		nextHead := e.published
		listChanges := len(w.subs) != 0
		skipped := nextHead != w.basePublication+1
		var recorded *world.ChangeSet
		var plan watchSpanPlan
		switch {
		case !skipped:
			recorded = e.watchedHeads[len(e.watchedHeads)-1].changes
		case listChanges:
			plan = e.watchSpanPlanLocked(w.basePublication, w.baseRefLocked().CloneVT(), nextHead)
		}
		locked.Unlock()

		// Resolve the span outside the publication lock. Every subscriber, and
		// so every view and every write's rehearsal, waits behind it, so any
		// fallback tree reads go ahead of other reads on a bounded link
		// (block.ReadUrgent), and a block only an absent peer could serve fails
		// instead of waiting for the peer.
		compareCtx, _ := block.WithoutPeerWait(block.WithReadClass(w.ctx, block.ReadUrgent))
		var keys []string
		var changes *world.ChangeSet
		var err error
		switch {
		case !skipped && recorded != nil && !recorded.Unknown:
			changes = recorded
			keys = recorded.Keys
			if keys == nil {
				keys = []string{}
			}
		case !skipped && recorded != nil:
			changes = recorded
			keys, err = e.compareWatchedObjectKeys(compareCtx, w, w.base, next)
		case !skipped:
			keys, changes, err = e.compareChangeWatchHeads(compareCtx, w, w.base, next, listChanges)
		case listChanges:
			changes, err = e.resolveWatchSpan(compareCtx, plan)
			if err == nil && !changes.Unknown {
				keys = changes.Keys
				if keys == nil {
					keys = []string{}
				}
			}
		default:
			changes = world.NewUnknownChangeSet()
		}
		if err != nil && compareCtx.Err() == nil {
			e.le.WithError(err).Warn("world change comparison failed; waking every watcher")
		}

		// Read the changed objects' revisions once for all their waiters.
		var revs map[string]objectKeyState
		if len(keys) != 0 {
			revs = e.readWatchedRevs(compareCtx, w, next, keys)
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
		w.wakeKeysLocked(keys, nextHead, revs)
		prevRelease := w.advanceLocked(next, nextSeqno, nextHead, nextRelease, changes, listChanges && err == nil)
		locked.Unlock()

		// Hand the span to its subscribers, then release the previous base.
		w.handOut(nextSeqno, changes)
		prevRelease()
	}
}

// compareChangeWatchHeads compares two held heads. keys lists the changed
// object records; a changed quad does not change the records it names. For a
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
		keys, err = e.compareWatchedObjectKeys(ctx, w, before, after)
		return keys, world.NewUnknownChangeSet(), err
	}

	// Add the changed quads for the subscribers.
	changes = world.NewUnknownChangeSet()
	if listChanges {
		changes, err = worldChangesWithKeys(ctx, beforeBcs, afterBcs, slices.Clone(keys), changeDiffLimit)
	}
	return keys, changes, err
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
	locked := e.bcast.Lock()
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
	locked := e.bcast.Lock()
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

// newEngineChangeWatch returns a watch under the Engine's bcast whose base is
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
// It records the span from the old base when listed reports its ChangeSet
// complete, and otherwise forgets every span, since a span without its
// ChangeSet breaks the chain from the earlier ones to the base. The caller
// must hold bcast.
func (w *engineChangeWatch) advanceLocked(
	next *bucket_lookup.Cursor,
	seqno, publication uint64,
	release func(),
	changes *world.ChangeSet,
	listed bool,
) func() {
	// Record the span, evicting the oldest at capacity.
	if !listed {
		w.history = nil
	} else {
		if len(w.history) == changeWatchHeads {
			w.history = slices.Delete(w.history, 0, 1)
		}
		w.history = append(w.history, changeSpan{
			after:    w.baseSeqno,
			afterRef: w.base.GetRef().CloneVT(),
			seqno:    seqno,
			changes:  changes,
		})
	}

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
