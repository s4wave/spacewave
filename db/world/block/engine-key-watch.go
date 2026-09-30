package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
)

// keyWatchDiffLimit bounds the changed keys one head comparison reports.
// A larger change wakes every waiter, which then rereads its object.
const keyWatchDiffLimit = 4096

// engineKeyWatch wakes object revision waiters only when a published head
// changes their object. It compares each new head with the last compared head
// once for all waiters. A changed object record never returns to an earlier
// value, because its revision grows with every change, so comparing the last
// compared head with the newest one covers every head published between them.
// The Engine starts the watch for the first waiter and it exits when the last
// waiter leaves or the Engine closes. Its fields are guarded by the Engine
// bcast lock.
type engineKeyWatch struct {
	// ctx cancels a comparison in flight when the Engine closes.
	ctx context.Context
	// cancel cancels ctx.
	cancel context.CancelFunc
	// base is the last compared head. Waiters read heads at or after it.
	base *bucket_lookup.Cursor
	// baseRelease releases the retention pin on base.
	baseRelease func()
	// waiters maps each watched object key to its wake channel.
	waiters map[string]*objectKeyWake
	// idle closes when the last waiter leaves so the watch releases its base
	// without waiting for another head.
	idle chan struct{}
}

// objectKeyWake is the wake channel shared by the waiters of one object key.
type objectKeyWake struct {
	// ch closes when the object may have changed and is then replaced.
	ch chan struct{}
	// refs counts the waiters registered on the key.
	refs int
}

// WaitObjectRev waits until the object at key reaches rev and returns its
// revision. The key watch wakes the waiter only when a published head changes
// the object's record, so unrelated commits cost the waiter nothing.
func (e *Engine) WaitObjectRev(ctx context.Context, key string, rev uint64, ignoreNotFound bool) (uint64, error) {
	// Discover durable commits that preceded this wait even if their advisory
	// notification was lost. The coordinator head watch adopts later commits.
	if err := e.refreshReadHead(ctx); err != nil {
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

// watchObjectKeyLocked registers a waiter on key, starting the key watch from
// the published head when none runs. The caller must hold bcast.
func (e *Engine) watchObjectKeyLocked(key string) (*engineKeyWatch, *objectKeyWake, error) {
	// Refuse new waiters once Close has begun draining.
	if e.closed {
		return nil, nil, ErrEngineClosed
	}

	// Start the watch from the published head, retained while it is the base.
	w := e.keyWatch
	if w == nil {
		ctx, cancel := context.WithCancel(context.Background())
		base := e.head.root.Clone()
		release, err := block.PinRoot(ctx, e.writeBlockStore, base.GetRef().GetRootRef())
		if err != nil {
			cancel()
			base.Release()
			return nil, nil, err
		}
		w = &engineKeyWatch{
			ctx:         ctx,
			cancel:      cancel,
			base:        base,
			baseRelease: release,
			waiters:     make(map[string]*objectKeyWake),
			idle:        make(chan struct{}),
		}
		e.keyWatch = w
		e.keyWatches++
		go e.runKeyWatch(w)
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

// unwatchObjectKey removes a waiter registration from its key watch.
func (e *Engine) unwatchObjectKey(w *engineKeyWatch, key string, wake *objectKeyWake) {
	// Drop the waiter, and the key once no waiter shares it.
	locked := e.bcast.Lock()
	wake.refs--
	if wake.refs == 0 {
		delete(w.waiters, key)
	}

	// Let an idle watch release its base without waiting for another head.
	if len(w.waiters) == 0 {
		close(w.idle)
		w.idle = make(chan struct{})
	}
	locked.Unlock()
}

// runKeyWatch compares each newly published head with the last compared head
// and wakes the waiters of the changed objects. It exits once no waiter
// remains or the Engine closes.
func (e *Engine) runKeyWatch(w *engineKeyWatch) {
	for {
		// Stop when nobody waits or the Engine closes.
		locked := e.bcast.Lock()
		if e.closed || len(w.waiters) == 0 {
			e.stopKeyWatchLocked(w)
			locked.Unlock()
			w.cancel()
			w.baseRelease()
			w.base.Release()

			// Let Close release the stores only after the base is released.
			locked = e.bcast.Lock()
			e.keyWatches--
			locked.Broadcast()
			locked.Unlock()
			return
		}

		// Wait for a head the watch has not compared. A wakeup without a new
		// head reports an observation failure, which waiters must see.
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

		// Retain the new head while it is compared and while it is the base.
		next := e.head.root.Clone()
		nextRelease, err := block.PinRoot(w.ctx, e.writeBlockStore, next.GetRef().GetRootRef())
		locked.Unlock()
		if err != nil {
			nextRelease = func() {}
		}

		// Compare the object trees outside the publication lock.
		var changed []string
		complete := false
		if err == nil {
			changed, complete, err = e.changedObjectKeys(w.ctx, w.base, next)
		}
		if err != nil && w.ctx.Err() == nil {
			e.le.WithError(err).Warn("world key watch comparison failed; waking every waiter")
		}

		// Wake the changed objects' waiters, or all of them when the
		// comparison could not list the changes, then advance the base.
		locked = e.bcast.Lock()
		if complete {
			w.wakeKeysLocked(changed)
		} else {
			w.wakeAllLocked()
		}
		prev, prevRelease := w.base, w.baseRelease
		w.base, w.baseRelease = next, nextRelease
		locked.Unlock()
		prevRelease()
		prev.Release()
	}
}

// stopKeyWatchLocked detaches w so the next waiter starts a fresh watch, and
// wakes its waiters. The caller must hold bcast.
func (e *Engine) stopKeyWatchLocked(w *engineKeyWatch) {
	if e.keyWatch == w {
		e.keyWatch = nil
	}
	w.wakeAllLocked()
}

// changedObjectKeys compares the object trees of two World roots.
func (e *Engine) changedObjectKeys(ctx context.Context, before, after *bucket_lookup.Cursor) ([]string, bool, error) {
	_, beforeBcs := before.BuildTransactionWithStore(nil, e.writeBlockStore)
	_, afterBcs := after.BuildTransactionWithStore(nil, e.writeBlockStore)
	return ChangedObjectKeys(ctx, beforeBcs, afterBcs, keyWatchDiffLimit)
}

// wakeKeysLocked wakes the waiters of the listed keys. The caller must hold bcast.
func (w *engineKeyWatch) wakeKeysLocked(keys []string) {
	for _, key := range keys {
		if wake := w.waiters[key]; wake != nil {
			wake.signal()
		}
	}
}

// wakeAllLocked wakes every waiter. The caller must hold bcast.
func (w *engineKeyWatch) wakeAllLocked() {
	for _, wake := range w.waiters {
		wake.signal()
	}
}

// signal wakes the key's current waiters and arms a fresh channel.
func (k *objectKeyWake) signal() {
	close(k.ch)
	k.ch = make(chan struct{})
}
