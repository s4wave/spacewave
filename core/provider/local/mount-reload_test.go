package provider_local

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	space_sobject "github.com/s4wave/spacewave/core/space/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/world"
)

// TestMountReloadsAfterAmbiguousCommit fails a mounted write's commit, and
// then a replace's, after each stored its state, and checks that each reads
// as committed: the write returns nil and the same mount writes on, and the
// replace returns nil and a new mount shows it. A commit error that finds
// another writer's state stored instead returns ErrStateWriteUnknown and
// reloads. A commit error whose settle read fails, or finds a successor of
// the committed write, returns ErrStateWriteUnknown and reloads a state that
// holds the operation once.
func TestMountReloadsAfterAmbiguousCommit(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	_, _, a, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()
	errAmbiguous := errors.New("fake ambiguous commit")
	ref := createTestSharedObject(ctx, t, a, "ambiguous")
	so, rel := mountTestSharedObject(ctx, t, a, ref)
	defer rel()
	writeTestStateData(ctx, t, so, []byte("fake-a"))

	// A mounted write stores its state, then its commit fails.
	var once sync.Once
	testHookStateCommitted.Store(new(func(id string) error {
		var err error
		once.Do(func() { err = errAmbiguous })
		return err
	}))
	t.Cleanup(func() { testHookStateCommitted.Store(nil) })
	if err := tryWriteTestStateData(ctx, so, []byte("fake-b")); err != nil {
		t.Fatalf("the stored write returned %v", err)
	}
	if so.ctx.Err() != nil {
		t.Fatal("a stored write restarted the mount")
	}
	waitTestStateData(ctx, t, so, []byte("fake-b"))
	other, err := a.ReadStoredSharedObject(ctx, "ambiguous")
	if err != nil {
		t.Fatal(err)
	}
	writeTestStateData(ctx, t, so, []byte("fake-c"))

	// A commit error after another writer stored an earlier state reloads.
	testHookStateCommitted.Store(new(func(id string) error {
		err := a.runSoObjectStoreTx(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
			return storeTestStateData(ctx, tx, "ambiguous", other.State)
		})
		if err != nil {
			return err
		}
		return errAmbiguous
	}))
	if err := tryWriteTestStateData(ctx, so, []byte("fake-lost")); !errors.Is(err, errAmbiguous) || !errors.Is(err, sobject.ErrStateWriteUnknown) {
		t.Fatalf("the overwritten write returned %v", err)
	}
	testHookStateCommitted.Store(nil)
	expectTestMountEnded(ctx, t, so)
	next, relNext := mountTestSharedObject(ctx, t, a, ref)
	defer relNext()
	waitTestStateData(ctx, t, next, []byte("fake-b"))
	writeTestStateData(ctx, t, next, []byte("fake-d"))

	// A replace stores its state, then its commit fails.
	peer, err := a.vol.GetPeer(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := peer.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	testHookReplaceCommitted.Store(new(func(string) error { return errAmbiguous }))
	t.Cleanup(func() { testHookReplaceCommitted.Store(nil) })
	stored, err := a.ReadStoredSharedObject(ctx, "ambiguous")
	if err != nil {
		t.Fatal(err)
	}
	origin := ReplaceOrigin{Generation: stored.Entry.GetGeneration(), State: stored.State, Source: []byte("source")}
	if err := a.ReplaceSharedObjectState(ctx, a.le, "ambiguous", owner, []byte("fake-moved"), origin); err != nil {
		t.Fatalf("the stored replace returned %v", err)
	}
	expectTestMountEnded(ctx, t, next)
	moved, relMoved := mountTestSharedObject(ctx, t, a, ref)
	defer relMoved()
	waitTestStateData(ctx, t, moved, []byte("fake-moved"))
	writeTestStateData(ctx, t, moved, []byte("fake-e"))

	// An operation's write stores its state, then its commit fails and
	// storage cannot be read to settle it: the outcome is unknown, and the
	// mount reloads a state holding the operation exactly once.
	author := moved.GetPeerID().String()
	before := testAuthorNonce(ctx, t, moved, author)
	var commitOnce, settleOnce sync.Once
	testHookStateCommitted.Store(new(func(string) error {
		var err error
		commitOnce.Do(func() { err = errAmbiguous })
		return err
	}))
	testHookStateSettling.Store(new(func(string) error {
		var err error
		settleOnce.Do(func() { err = errors.New("fake settle read failure") })
		return err
	}))
	t.Cleanup(func() { testHookStateSettling.Store(nil) })
	if _, _, err := moved.QueueOperation(ctx, []byte("op-unknown")); !errors.Is(err, sobject.ErrStateWriteUnknown) {
		t.Fatalf("the unsettled operation returned %v", err)
	}
	testHookStateCommitted.Store(nil)
	testHookStateSettling.Store(nil)
	expectTestMountEnded(ctx, t, moved)
	reloaded, relReloaded := mountTestSharedObject(ctx, t, a, ref)
	defer relReloaded()
	if got := testAuthorNonce(ctx, t, reloaded, author); got != before+1 {
		t.Fatalf("the reloaded state holds the author at nonce %d, want %d", got, before+1)
	}

	// An operation's write stores its state, then its commit fails and
	// another writer stores a successor built on it before the settle read:
	// the outcome is unknown, and the reloaded state holds the operation
	// once. The successor is the committed bytes with an unknown field
	// appended. The successor decodes to the same state.
	before = testAuthorNonce(ctx, t, reloaded, author)
	commitOnce = sync.Once{}
	testHookStateCommitted.Store(new(func(string) error {
		var err error
		commitOnce.Do(func() {
			err = a.runSoObjectStoreTx(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
				data, _, err := ReadSOStateData(ctx, tx, "ambiguous")
				if err != nil {
					return err
				}
				return storeTestStateData(ctx, tx, "ambiguous", append(data, 0x98, 0x06, 0x01))
			})
			if err == nil {
				err = errAmbiguous
			}
		})
		return err
	}))
	if _, _, err := reloaded.QueueOperation(ctx, []byte("op-successor")); !errors.Is(err, sobject.ErrStateWriteUnknown) {
		t.Fatalf("the operation under a successor returned %v", err)
	}
	testHookStateCommitted.Store(nil)
	expectTestMountEnded(ctx, t, reloaded)
	last, relLast := mountTestSharedObject(ctx, t, a, ref)
	defer relLast()
	if got := testAuthorNonce(ctx, t, last, author); got != before+1 {
		t.Fatalf("the state after the successor holds the author at nonce %d, want %d", got, before+1)
	}
}

// TestReloadFenceRefusesPostponedWrites ends an operation's write unknown
// with its state lost while the reload's backoff postpones the restart, then
// tries to commit at the old base in that window: an operation and a state
// write on the same handle. Each fails with ErrMountReloading and stores
// nothing, and the restarted mount holds the author at the nonce before the
// unknown write. Without the fence the compare-and-set passes, since storage
// still holds the token the mount loaded, and the operation commits at the
// old base.
func TestReloadFenceRefusesPostponedWrites(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	_, _, a, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()
	ref := createTestSharedObject(ctx, t, a, "fenced")
	so, rel := mountTestSharedObject(ctx, t, a, ref)
	defer rel()
	writeTestStateData(ctx, t, so, []byte("fake-a"))
	author := so.GetPeerID().String()
	before := testAuthorNonce(ctx, t, so, author)

	// Postpone the next reload by the longest backoff.
	tracker, ok := a.sobjects.GetKey("fenced")
	if !ok {
		t.Fatal("the mounted object has no tracker")
	}
	tracker.reloadMtx.Lock()
	tracker.reloadLast, tracker.reloadDelay = time.Now(), reloadBackoffMax
	tracker.reloadMtx.Unlock()

	// An operation's commit fails without storing it, and storage cannot be
	// read to settle it.
	var commitOnce, settleOnce sync.Once
	testHookStateCommit.Store(new(func(string, *sobject.SOState, *sobject.SOState) error {
		var err error
		commitOnce.Do(func() { err = errors.New("fake lost commit") })
		return err
	}))
	t.Cleanup(func() { testHookStateCommit.Store(nil) })
	testHookStateSettling.Store(new(func(string) error {
		var err error
		settleOnce.Do(func() { err = errors.New("fake settle read failure") })
		return err
	}))
	t.Cleanup(func() { testHookStateSettling.Store(nil) })
	if _, _, err := so.QueueOperation(ctx, []byte("op-unknown")); !errors.Is(err, sobject.ErrStateWriteUnknown) {
		t.Fatalf("the unsettled operation returned %v", err)
	}
	testHookStateCommit.Store(nil)
	testHookStateSettling.Store(nil)

	// Before the postponed restart, writes at the old base are refused.
	if so.ctx.Err() != nil {
		t.Fatal("the reload restarted the mount without its backoff")
	}
	if _, _, err := so.QueueOperation(ctx, []byte("op-stale")); !errors.Is(err, sobject.ErrMountReloading) {
		t.Fatalf("an operation at the old base returned %v", err)
	}
	if err := tryWriteTestStateData(ctx, so, []byte("fake-stale")); !errors.Is(err, sobject.ErrMountReloading) {
		t.Fatalf("a state write at the old base returned %v", err)
	}

	// The restarted mount holds neither, and takes writes.
	expectTestMountEnded(ctx, t, so)
	next, relNext := mountTestSharedObject(ctx, t, a, ref)
	defer relNext()
	waitTestStateData(ctx, t, next, []byte("fake-a"))
	if got := testAuthorNonce(ctx, t, next, author); got != before {
		t.Fatalf("the restarted mount holds the author at nonce %d, want %d", got, before)
	}
	writeTestStateData(ctx, t, next, []byte("fake-b"))
}

// TestWaitSettledAfterUnknownCommit mounts a Space body through the space
// sobject controller, as the product does, and ends four of its World commits
// unknown in turn, each with the settle read failing: two after the commit
// stored the write and two in place of the commit. Each unknown reloads the
// mount, and the body's getter starts the World engine again. Each commit
// returns world.DurabilityUnknownError with the operation's hash, and
// WaitSettled on the body's engine waits for the restarted mount's engine. It
// must report applied exactly when the World holds the write's object. A
// report of applied for a lost write, a lost settle (ErrNoSettle or a hang)
// after a reload, or a lost write the World holds all fail. Each time the
// client then writes again.
func TestWaitSettledAfterUnknownCommit(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	tb, _, a, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()
	tb.StaticResolver.AddFactory(space_sobject.NewFactory(tb.Bus))
	_, ctrlRef, err := tb.Bus.AddDirective(ctx, resolver.NewLoadControllerWithConfig(&space_sobject.Config{}), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ctrlRef.Release()
	const id = "settle-space"
	meta, err := space.NewSharedObjectMeta("Settle Space")
	if err != nil {
		t.Fatal(err)
	}
	ref, err := a.CreateSharedObject(ctx, id, meta, "", "")
	if err != nil {
		t.Fatal(err)
	}
	mounted, bodyRef, err := space.ExMountSpaceSoBody(ctx, tb.Bus, ref, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer bodyRef.Release()
	eng := mounted.GetSharedObjectBody().GetWorldEngine()
	waiter, ok := eng.(world.SettleWaiter)
	if !ok {
		t.Fatalf("the Space body's engine %T cannot settle", eng)
	}

	// create commits one object in its own transaction. has reads it.
	create := func(key string) error {
		tx, err := eng.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		defer tx.Discard()
		obj, _, err := world.CreateWorldObject(ctx, tx, key, func(cursor *block.Cursor) error {
			cursor.SetBlock(block_mock.NewExample("settle wrote "+key), true)
			return nil
		})
		world.ReleaseObjectState(obj)
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	has := func(key string) bool {
		tx, err := eng.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Discard()
		obj, found, err := tx.GetObject(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		world.ReleaseObjectState(obj)
		return found
	}
	if err := create("before"); err != nil {
		t.Fatal(err)
	}

	for i, landed := range []bool{true, false, false, true} {
		key := fmt.Sprintf("%s-%d", map[bool]string{true: "landed", false: "lost"}[landed], i)

		// Fail the state write that adds the World operation, after its
		// commit stored it or in place of the commit, and the settle read.
		var commitOnce, settleOnce sync.Once
		var stored atomic.Bool
		testHookStateCommit.Store(new(func(sid string, prev, next *sobject.SOState) error {
			var err error
			if sid == id && addsTestWorldOp(prev, next) {
				commitOnce.Do(func() {
					if landed {
						stored.Store(true)
					} else {
						err = errors.New("fake lost commit")
					}
				})
			}
			return err
		}))
		testHookStateCommitted.Store(new(func(string) error {
			if stored.Swap(false) {
				return errors.New("fake ambiguous commit")
			}
			return nil
		}))
		testHookStateSettling.Store(new(func(sid string) error {
			var err error
			if sid == id {
				settleOnce.Do(func() { err = errors.New("fake settle read failure") })
			}
			return err
		}))
		err := create(key)
		testHookStateCommit.Store(nil)
		testHookStateCommitted.Store(nil)
		testHookStateSettling.Store(nil)
		var unknown *world.DurabilityUnknownError
		if !errors.As(err, &unknown) || len(unknown.OpHash) == 0 {
			t.Fatalf("the %s commit returned %v, want a durability unknown with its hash", key, err)
		}

		// A write during the restart backoff waits for the old engine to
		// retire and lands on its successor, instead of using up its
		// retries on the old one. With the fence off the old mount takes
		// it, or refuses it with the state it changed, as before.
		during := "during-" + key
		if err := create(during); err != nil && !sobject.ReloadFenceOff {
			t.Fatalf("writing during the %s reload: %v", key, err)
		}

		// The settle reports the outcome the World holds.
		head, applied, err := waiter.WaitSettled(ctx, unknown.OpHash)
		if err != nil {
			t.Fatalf("settling the %s commit: %v", key, err)
		}
		if applied != landed || head.Root == nil {
			t.Fatalf("the %s commit settled applied=%t at head %v, want applied=%t", key, applied, head, landed)
		}
		if got := has(key); got != landed {
			t.Fatalf("the World holds the %s object: %t, want %t", key, got, landed)
		}
		if !sobject.ReloadFenceOff && !has(during) {
			t.Fatalf("the World lacks the %s object", during)
		}
		if err := create("after-" + key); err != nil {
			t.Fatalf("writing after the %s settle: %v", key, err)
		}
	}
	t.Cleanup(func() {
		testHookStateCommit.Store(nil)
		testHookStateCommitted.Store(nil)
		testHookStateSettling.Store(nil)
	})
}

// TestMountReloadsAfterStoreChange changes the stored state behind a mount
// and checks that the next write fails with ErrStateChanged once and reloads
// the mount in one new run. The reloaded mount then takes writes.
func TestMountReloadsAfterStoreChange(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	_, _, a, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()
	ref := createTestSharedObject(ctx, t, a, "behind")
	so, rel := mountTestSharedObject(ctx, t, a, ref)
	defer rel()
	writeTestStateData(ctx, t, so, []byte("fake-a"))
	first, err := a.ReadStoredSharedObject(ctx, "behind")
	if err != nil {
		t.Fatal(err)
	}
	writeTestStateData(ctx, t, so, []byte("fake-b"))

	// Store the earlier state behind the mount, and count the runs after it.
	err = a.runSoObjectStoreTx(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
		return storeTestStateData(ctx, tx, "behind", first.State)
	})
	if err != nil {
		t.Fatal(err)
	}
	var mtx sync.Mutex
	var runs int
	testHookRunStarting.Store(new(func(id string) {
		if id == "behind" {
			mtx.Lock()
			runs++
			mtx.Unlock()
		}
	}))
	t.Cleanup(func() { testHookRunStarting.Store(nil) })

	// The next write fails once and the mount reloads the stored state.
	if err := tryWriteTestStateData(ctx, so, []byte("fake-c")); !errors.Is(err, ErrStateChanged) {
		t.Fatalf("a write behind a store change returned %v", err)
	}
	expectTestMountEnded(ctx, t, so)
	next, relNext := mountTestSharedObject(ctx, t, a, ref)
	defer relNext()
	waitTestStateData(ctx, t, next, []byte("fake-a"))

	mtx.Lock()
	n := runs
	mtx.Unlock()
	if n != 1 {
		t.Fatalf("the store change started %d runs, want 1", n)
	}
	writeTestStateData(ctx, t, next, []byte("fake-c"))
}

// TestRestartLoadWaitsForOldWriter holds a write of a mount inside its
// transaction, past its mount check, restarts the mount, and lets the write
// commit once the new run starts. The new run's load waits for the write, so
// the new mount shows it without any further write.
//
// The hooks order the write's check, the restart and the new run's start.
// Whether the new run's load has begun when the write is let go is not
// observable, so the test cannot force the load ahead of the commit.
func TestRestartLoadWaitsForOldWriter(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	_, _, a, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()
	ref := createTestSharedObject(ctx, t, a, "handoff")
	so, rel := mountTestSharedObject(ctx, t, a, ref)
	defer rel()
	writeTestStateData(ctx, t, so, []byte("fake-a"))

	// Hold the next write past its mount check.
	reached, resume := make(chan struct{}), make(chan struct{})
	var once sync.Once
	testHookStateWriting.Store(new(func(id string) {
		if id != "handoff" {
			return
		}
		once.Do(func() {
			close(reached)
			<-resume
		})
	}))
	t.Cleanup(func() { testHookStateWriting.Store(nil) })
	writeErr := make(chan error, 1)
	go func() {
		writeErr <- tryWriteTestStateData(ctx, so, []byte("fake-late"))
	}()
	select {
	case <-reached:
	case <-ctx.Done():
		t.Fatal("the write did not reach its transaction")
	}

	// Restart the mount and let the write commit once the new run starts.
	started := make(chan struct{})
	var startOnce sync.Once
	testHookRunStarting.Store(new(func(id string) {
		if id == "handoff" {
			startOnce.Do(func() { close(started) })
		}
	}))
	t.Cleanup(func() { testHookRunStarting.Store(nil) })
	a.restartSharedObject("handoff", nil)
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		close(resume)
		t.Fatal("the new run did not start while the old run's write was open")
	}
	close(resume)
	if err := <-writeErr; err != nil {
		t.Fatalf("the held write returned %v", err)
	}

	// The new mount shows the held write and takes the next one.
	next, relNext := mountTestSharedObject(ctx, t, a, ref)
	defer relNext()
	if got := readTestStateData(ctx, t, next); !bytes.Equal(got, []byte("fake-late")) {
		t.Fatalf("the restarted mount reads %q, want the held write", got)
	}
	writeTestStateData(ctx, t, next, []byte("fake-b"))
}

// TestRestartDropsPublishedMount holds the run a create or a delete starts
// on a held tracker, and checks that the restart cleared the old run's result
// first: until the new run publishes, a waiter gets neither the refusal
// published before the create nor the handle ended by the delete.
func TestRestartDropsPublishedMount(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	_, _, a, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()

	// Hold the tracker of an unlisted id and let it publish its refusal.
	providerID := a.t.accountInfo.GetProviderId()
	accountID := a.t.accountInfo.GetProviderAccountId()
	ref := sobject.NewSharedObjectRef(providerID, accountID, "held", SobjectBlockStoreID("held"))
	tkrRef, tkr, _ := a.sobjects.AddKeyRef("held")
	defer tkrRef.Release()
	tkr.ref.SetResult(ref, nil)
	expectTestMountNotFound(ctx, t, a, ref)

	// Create the object while its next run is held: the refusal is gone.
	resume := holdTestRunStart(t, "held")
	ref = createTestSharedObject(ctx, t, a, "held")
	expectTestResultCleared(ctx, t, tkr)
	resume()
	_, rel := mountTestSharedObject(ctx, t, a, ref)
	defer rel()

	// Delete it while its next run is held: the ended handle is gone.
	resume = holdTestRunStart(t, "held")
	if err := a.DeleteSharedObject(ctx, "held"); err != nil {
		t.Fatal(err)
	}
	expectTestResultCleared(ctx, t, tkr)
	resume()
	expectTestMountNotFound(ctx, t, a, ref)
}

// TestRestartChainKeepsNewestMount holds a ready run's exit while a delete
// and a transfer start two more runs, so the third publishes while the first
// is still exiting, and checks that the first run's exit leaves the third
// run's result in place.
func TestRestartChainKeepsNewestMount(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	_, _, a, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()

	// Mount the object and hold its tracker.
	ref := createTestSharedObject(ctx, t, a, "chain")
	first, rel := mountTestSharedObject(ctx, t, a, ref)
	defer rel()
	tkrRef, tkr, _ := a.sobjects.AddKeyRef("chain")
	defer tkrRef.Release()

	// Hold the first run's exit before it clears its result.
	reached, resume, exited := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	testHookRunEnding.Store(new(func(id string, end func()) {
		held := false
		if id == "chain" {
			once.Do(func() { held = true })
		}
		if !held {
			end()
			return
		}
		close(reached)
		<-resume
		end()
		close(exited)
	}))
	t.Cleanup(func() { testHookRunEnding.Store(nil) })
	var resumeOnce sync.Once
	release1 := func() { resumeOnce.Do(func() { close(resume) }) }
	t.Cleanup(release1)

	// Delete the object, and wait for the first run to reach its exit.
	if err := a.DeleteSharedObject(ctx, "chain"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-reached:
	case <-ctx.Done():
		t.Fatal("the first run did not exit after the delete")
	}

	// Create the object again and transfer state in, restarting the tracker
	// past the second run, and wait for the third run to publish.
	createTestSharedObject(ctx, t, a, "chain")
	peer, err := a.vol.GetPeer(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := peer.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	replaceTestState(ctx, t, a, "chain", owner, []byte("fake-transfer"))
	actx, acancel := context.WithTimeout(ctx, 10*time.Second)
	defer acancel()
	third, err := tkr.res.await(actx)
	if err != nil {
		t.Fatalf("the third run returned %v", err)
	}
	if third == first {
		t.Fatal("the tracker still holds the first run's handle")
	}

	// Let the first run clear its result and finish exiting.
	release1()
	select {
	case <-exited:
	case <-ctx.Done():
		t.Fatal("the first run did not finish exiting")
	}

	// A mount still gets the third run's object.
	mctx, mcancel := context.WithTimeout(ctx, 10*time.Second)
	defer mcancel()
	so, relSO, err := a.MountSharedObject(mctx, ref, nil)
	if err != nil {
		t.Fatalf("a mount after the first run exited returned %v", err)
	}
	defer relSO()
	if so != third {
		t.Fatal("the mount returned a handle other than the third run's")
	}
	if got := readTestStateData(ctx, t, third); !bytes.Equal(got, []byte("fake-transfer")) {
		t.Fatalf("the transferred object reads %q", got)
	}
}
