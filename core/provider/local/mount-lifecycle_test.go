package provider_local

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
)

// TestReplaceSharedObjectRacingHandleWrite races a mounted handle's state
// writes against a replace from both sides. A write acknowledged after the
// caller read the state makes the replace refuse, so the write is not lost. A
// write between the replace's commit and its restart of the mount fails, so
// the replaced lineage does not come back.
func TestReplaceSharedObjectRacingHandleWrite(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	_, _, a, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()
	peer, err := a.vol.GetPeer(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := peer.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Mount the object, read it as a transfer does, then acknowledge a write.
	ref := createTestSharedObject(ctx, t, a, "replaced")
	so, rel := mountTestSharedObject(ctx, t, a, ref)
	defer rel()
	read := func() ReplaceOrigin {
		stored, err := a.ReadStoredSharedObject(ctx, "replaced")
		if err != nil {
			t.Fatal(err)
		}
		return ReplaceOrigin{Generation: stored.Entry.GetGeneration(), State: stored.State, Source: []byte("source")}
	}
	stale := read()
	writeTestStateData(ctx, t, so, []byte("fake-acked"))

	// A replace planned before the acknowledged write refuses and keeps it.
	if err := a.ReplaceSharedObjectState(ctx, a.le, "replaced", owner, []byte("fake-moved"), stale); !errors.Is(err, ErrStateChanged) {
		t.Fatalf("a replace over an acknowledged write returned %v", err)
	}
	if got := readTestStateData(ctx, t, so); !bytes.Equal(got, []byte("fake-acked")) {
		t.Fatalf("the acknowledged write reads %q after the refused replace", got)
	}

	// Write through the live handle after the replace commits, before it
	// restarts the mount.
	var hookErr error
	var ranHook bool
	testHookReplaceCommitted.Store(new(func(id string) error {
		if id != "replaced" {
			return nil
		}
		ranHook = true
		if so.ctx.Err() != nil {
			t.Error("the mount ended before the replace restarted it")
		}
		hookErr = tryWriteTestStateData(ctx, so, []byte("fake-resurrected"))
		return nil
	}))
	t.Cleanup(func() { testHookReplaceCommitted.Store(nil) })
	if err := a.ReplaceSharedObjectState(ctx, a.le, "replaced", owner, []byte("fake-moved"), read()); err != nil {
		t.Fatal(err)
	}

	// The racing write failed, storage holds the replace, and a new mount
	// reads it.
	if !ranHook {
		t.Fatal("the replace did not reach its commit hook")
	}
	if !errors.Is(hookErr, ErrStateChanged) {
		t.Fatalf("a write racing the replace returned %v, want ErrStateChanged", hookErr)
	}
	stored, err := a.ReadStoredSharedObject(ctx, "replaced")
	if err != nil {
		t.Fatal(err)
	}
	if !stored.Unchanged() {
		t.Fatal("the stored state is not the one the replace wrote")
	}
	mctx, mcancel := context.WithTimeout(ctx, 10*time.Second)
	defer mcancel()
	next, relNext, err := a.MountSharedObject(mctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relNext()
	if got := readTestStateData(ctx, t, next.(*SharedObject)); !bytes.Equal(got, []byte("fake-moved")) {
		t.Fatalf("the replaced object reads %q", got)
	}
}

// TestRefusedMountRevivesOnCreate holds a mount between reading that its
// object has no state and publishing the refusal, creates the object there,
// and checks that the tracker restarts and mounts the new object while the
// first reference still holds the tracker.
func TestRefusedMountRevivesOnCreate(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	_, _, a, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()

	// Hold the first refusal of the unlisted id.
	reached, resume := make(chan struct{}), make(chan struct{})
	var once sync.Once
	testHookStateRefused.Store(new(func(id string) {
		if id != "revive" {
			return
		}
		once.Do(func() {
			close(reached)
			<-resume
		})
	}))
	t.Cleanup(func() { testHookStateRefused.Store(nil) })

	// Take a reference, as MountSharedObject does, and wait for the read.
	providerID := a.t.accountInfo.GetProviderId()
	accountID := a.t.accountInfo.GetProviderAccountId()
	ref := sobject.NewSharedObjectRef(providerID, accountID, "revive", SobjectBlockStoreID("revive"))
	tkrRef, tkr, _ := a.sobjects.AddKeyRef("revive")
	defer tkrRef.Release()
	tkr.ref.SetResult(ref, nil)
	select {
	case <-reached:
	case <-ctx.Done():
		t.Fatal("the mount did not read the missing state")
	}

	// Create the object, then let the refusal go on.
	createTestSharedObject(ctx, t, a, "revive")
	close(resume)

	// The held tracker mounts the created object.
	mctx, mcancel := context.WithTimeout(ctx, 10*time.Second)
	defer mcancel()
	so, err := tkr.res.await(mctx)
	if err != nil {
		t.Fatalf("the tracker held across the create returned %v", err)
	}
	if got := readTestStateData(ctx, t, so); len(got) != 0 {
		t.Fatalf("the created object reads %q", got)
	}
}

// TestRetiredLocalWriteCannotCrossRecreate opens the real account writer only
// after a delete and same-id recreate have completed.
func TestRetiredLocalWriteCannotCrossRecreate(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	_, _, a, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()
	const id = "gated-relist"
	old, releaseOld := mountTestSharedObject(ctx, t, a, createTestSharedObject(ctx, t, a, id))
	defer releaseOld()

	// Wrap only this write's copy of the mount store. Leave the real mount
	// and every other user of the account store alone.
	store := old.objStore.(mountStore)
	gate := &mountOpenGate{ObjectStore: store.ObjectStore, write: true, entered: make(chan struct{}), resume: make(chan struct{})}
	store.ObjectStore = gate
	gated := &SharedObject{ctx: old.ctx, tkr: old.tkr, objStore: store}
	writeCtx, cancelWrite := context.WithCancel(ctx)
	defer cancelWrite()
	result := make(chan error, 1)
	go func() { result <- tryWriteTestLocalKeys(writeCtx, gated, 1) }()
	select {
	case <-gate.entered:
	case <-ctx.Done():
		t.Fatal("the old write did not reach the account writer boundary")
	}

	if err := a.DeleteSharedObject(ctx, id); err != nil {
		t.Fatal(err)
	}
	fresh, releaseFresh := mountTestSharedObject(ctx, t, a, createTestSharedObject(ctx, t, a, id))
	defer releaseFresh()
	if old.ctx.Err() == nil || fresh == old {
		t.Fatal("recreate did not retire the old mount")
	}
	local, releaseLocal, err := fresh.AccessLocalStateStore(ctx, "fake-store", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseLocal()
	key, want := []byte("fake-key-0000"), []byte("new mount's value")
	if err := kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
		return local.NewTransaction(ctx, true)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		return tx.Set(ctx, key, want)
	}); err != nil {
		t.Fatal(err)
	}
	close(gate.resume)
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) || writeCtx.Err() != nil {
			t.Errorf("old write returned %v with caller error %v, want mount cancellation only", err, writeCtx.Err())
		}
	case <-ctx.Done():
		t.Fatal("the old write did not return")
	}

	// Take the writer again to check that the refusal did not retain it.
	tx, err := local.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	got, found, err := tx.Get(ctx, key)
	if err != nil || !found || !bytes.Equal(got, want) {
		t.Fatalf("fresh value = %q (found %t, err %v), want %q", got, found, err, want)
	}
}

// TestRetiredLocalReadCannotCrossRecreate pauses the old read before the
// store can pin a snapshot of the new lifetime.
func TestRetiredLocalReadCannotCrossRecreate(t *testing.T) {
	// Retain the first mount and give the test's read its own gate.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	_, _, a, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()
	const id, storeID = "gated-read", "fake-store"
	key := []byte("fake-key-0000")
	old, releaseOld := mountTestSharedObject(ctx, t, a, createTestSharedObject(ctx, t, a, id))
	defer releaseOld()
	store := old.objStore.(mountStore)
	gate := &mountOpenGate{ObjectStore: store.ObjectStore, entered: make(chan struct{}), resume: make(chan struct{})}
	store.ObjectStore = gate
	gated := &SharedObject{ctx: old.ctx, tkr: old.tkr, objStore: store}
	local, releaseLocal, err := gated.AccessLocalStateStore(ctx, storeID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseLocal()

	// Start the old read with a caller that stays live through retirement.
	readCtx, cancelRead := context.WithCancel(ctx)
	defer cancelRead()
	type readResult struct {
		value []byte
		found bool
		err   error
	}
	result := make(chan readResult, 1)
	go func() {
		// Read the value if the mount admits the transaction.
		tx, err := local.NewTransaction(readCtx, false)
		if err != nil {
			result <- readResult{err: err}
			return
		}
		defer tx.Discard()
		value, found, err := tx.Get(readCtx, key)
		result <- readResult{value: bytes.Clone(value), found: found, err: err}
	}()
	select {
	case <-gate.entered:
	case <-ctx.Done():
		t.Fatal("the old read did not reach the snapshot boundary")
	}

	// Commit a fresh local value before the old read opens its snapshot.
	if err := a.DeleteSharedObject(ctx, id); err != nil {
		t.Fatal(err)
	}
	fresh, releaseFresh := mountTestSharedObject(ctx, t, a, createTestSharedObject(ctx, t, a, id))
	defer releaseFresh()
	if old.ctx.Err() == nil || fresh == old {
		t.Fatal("recreate did not retire the old mount")
	}
	if err := tryWriteTestLocalKeys(ctx, fresh, 1); err != nil {
		t.Fatal(err)
	}
	close(gate.resume)

	// The old read must not return any value from that snapshot.
	select {
	case got := <-result:
		if !errors.Is(got.err, context.Canceled) || got.found || got.value != nil || readCtx.Err() != nil {
			t.Fatalf("old read returned %q (found %t, err %v, caller err %v), want mount cancellation only", got.value, got.found, got.err, readCtx.Err())
		}
	case <-ctx.Done():
		t.Fatal("the old read did not return")
	}
}

// TestRetiredHostCannotReadRecreatedHistory asks a retained SOHost for signed
// entries and a suffix that are present in the recreated object's history.
func TestRetiredHostCannotReadRecreatedHistory(t *testing.T) {
	// Keep the old host across a delete and same-id create.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	_, _, a, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()
	const id = "retired-history"
	old, releaseOld := mountTestSharedObject(ctx, t, a, createTestSharedObject(ctx, t, a, id))
	defer releaseOld()
	oldHost := old.GetSOHost()
	if err := a.DeleteSharedObject(ctx, id); err != nil {
		t.Fatal(err)
	}
	fresh, releaseFresh := mountTestSharedObject(ctx, t, a, createTestSharedObject(ctx, t, a, id))
	defer releaseFresh()
	if old.ctx.Err() == nil || fresh.GetSOHost() == oldHost {
		t.Fatal("recreate did not retire the old host")
	}

	// Give the new host a retained transition so both history routes read rows.
	host := fresh.GetSOHost()
	state, err := host.GetHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cfg := state.GetConfig()
	genesis := cfg.GetConfigChainHash()
	change, err := sobject.BuildSOConfigChange(id, cfg, cfg, sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_INVITE, fresh.GetPrivKey(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := host.ApplyConfigChange(ctx, change, nil); err != nil {
		t.Fatal(err)
	}
	head, err := sobject.HashSOConfigChange(change)
	if err != nil {
		t.Fatal(err)
	}

	// First prove the new host can read the signed entry and suffix.
	if entry, err := host.ReadConfigEntry(ctx, genesis); err != nil || entry == nil {
		t.Fatalf("new host did not read its genesis: %v", err)
	}
	if suffix, err := host.ReadConfigHistory(ctx, genesis, head); err != nil || len(suffix) != 1 || !suffix[0].EqualVT(change) {
		t.Fatalf("new host returned %d history entries, err %v; want the retained transition", len(suffix), err)
	}

	// Neither history route on the old host may open a fresh snapshot.
	if entry, err := oldHost.ReadConfigEntry(ctx, genesis); !errors.Is(err, context.Canceled) || entry != nil {
		t.Errorf("old host returned a genesis entry: %t, err %v; want mount cancellation", entry != nil, err)
	}
	if suffix, err := oldHost.ReadConfigHistory(ctx, genesis, head); !errors.Is(err, context.Canceled) || len(suffix) != 0 {
		t.Errorf("old host returned %d history entries, err %v; want mount cancellation", len(suffix), err)
	}
}

// TestMountWaitEndsWithAccount requires account shutdown to end a pending
// mount without a publisher or caller cancellation, and refuse a later mount.
func TestMountWaitEndsWithAccount(t *testing.T) {
	// Give the account its real tracker and a promise that never resolves.
	// Do not start a Session or let a background mount publish a result.
	accountCtx, stopAccount := context.WithCancel(t.Context())
	defer stopAccount()
	const pendingID, untrackedID = "pending-at-stop", "untracked-at-stop"
	waiting := make(chan struct{})
	a := &ProviderAccount{lifecycleCtx: accountCtx}
	a.sobjects = keyed.NewKeyedRefCount(func(id string) (keyed.Routine, *sobjectTracker) {
		// Install the pending result before AddKeyRef exposes the tracker.
		_, tracker := a.buildSharedObjectTracker(id)
		if id == pendingID {
			tracker.res.prom.SetPromise(&mountWaitPromise{PromiseLike: promise.NewPromise[*SharedObject](), waiting: waiting})
		}
		return nil, tracker
	})
	a.sobjects.SetContext(accountCtx, true)
	defer a.sobjects.ClearContext()

	// Keep the caller live independently of both the account and the test's
	// failure deadline, and always release an unexpected successful mount.
	caller, cancelCaller := context.WithCancel(t.Context())
	defer cancelCaller()
	limit, cancelLimit := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancelLimit()
	startMount := func(id string) <-chan error {
		ref := sobject.NewSharedObjectRef("local", "test-account", id, SobjectBlockStoreID(id))
		result := make(chan error, 1)
		go func() {
			_, release, err := a.MountSharedObject(caller, ref, nil)
			if release != nil {
				release()
			}
			result <- err
		}()
		return result
	}
	checkMount := func(id string, result <-chan error) {
		t.Helper()
		select {
		case err := <-result:
			if !errors.Is(err, context.Canceled) || caller.Err() != nil {
				t.Errorf("mount %s returned %v with caller error %v, want account cancellation only", id, err, caller.Err())
			}
		case <-limit.Done():
			_, retained := a.sobjects.GetKey(id)
			t.Fatalf("mount %s did not return on account stop (tracker retained: %t)", id, retained)
		}
		if _, retained := a.sobjects.GetKey(id); retained {
			t.Errorf("mount %s retained a tracker after returning", id)
		}
	}

	// Stop only after the caller has entered the unresolved result's wait.
	result := startMount(pendingID)
	select {
	case <-waiting:
	case err := <-result:
		t.Fatalf("mount returned before waiting for its pending result: %v", err)
	case <-limit.Done():
		t.Fatal("mount did not enter the result wait")
	}
	if _, retained := a.sobjects.GetKey(pendingID); !retained {
		t.Fatal("pending mount did not retain its tracker")
	}
	stopAccount()
	a.sobjects.ClearContext()
	checkMount(pendingID, result)

	// A new call on the stopped account has no result or producer either.
	if _, retained := a.sobjects.GetKey(untrackedID); retained {
		t.Fatal("the second object already had a tracker before the mount")
	}
	checkMount(untrackedID, startMount(untrackedID))
}

// TestMountWaiterDiscardsRetiredResult holds an in-flight mount after it
// captures the ready promise, then deletes and recreates the same id.
func TestMountWaiterDiscardsRetiredResult(t *testing.T) {
	// Retain the first generation and gate the promise one caller captures.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	_, _, a, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()
	const id = "captured-mount"
	ref := createTestSharedObject(ctx, t, a, id)
	old, releaseOld := mountTestSharedObject(ctx, t, a, ref)
	defer releaseOld()
	res := old.tkr.res
	res.mtx.Lock()
	original, _ := res.prom.GetPromise()
	gate := &mountCapturePromise{PromiseLike: original, captured: make(chan struct{}), resume: make(chan struct{})}
	res.prom.SetPromise(gate)
	res.mtx.Unlock()

	// Pause one mount while its captured result is still the first generation.
	waiter := context.WithValue(ctx, mountCaptureKey{}, true)
	result := make(chan error, 1)
	mounted := make(chan sobject.SharedObject, 1)
	go func() {
		so, release, err := a.MountSharedObject(waiter, ref, nil)
		if err == nil {
			defer release()
		}
		mounted <- so
		result <- err
	}()
	select {
	case <-gate.captured:
	case <-ctx.Done():
		t.Fatal("mount did not capture the old promise")
	}

	// Retire the old promise and publish the new mount before selecting the old one.
	if err := a.DeleteSharedObject(ctx, id); err != nil {
		t.Fatal(err)
	}
	createTestSharedObject(ctx, t, a, id)
	fresh, releaseFresh := mountTestSharedObject(ctx, t, a, ref)
	defer releaseFresh()
	if fresh == old || old.ctx.Err() == nil {
		t.Fatal("recreate did not retire the captured mount")
	}
	close(gate.resume)

	// A successful mount must return the new live handle, not the ended one.
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
		if got := <-mounted; got != fresh {
			t.Fatal("mount returned the retired handle")
		}
	case <-ctx.Done():
		t.Fatal("mount did not return the recreated result")
	}
}

// TestRetiredLoadKeepsPlaceholderRevive holds a completed load across two
// restarts. The old load must not clear the new run's refusal to be revived.
func TestRetiredLoadKeepsPlaceholderRevive(t *testing.T) {
	// Keep the tracker retained even while there is no mounted handle.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	_, _, a, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()
	const id = "retired-load"
	ref := createTestSharedObject(ctx, t, a, id)
	_, releaseHealth, err := a.AccessSharedObjectHealth(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseHealth()
	old, releaseOld := mountTestSharedObject(ctx, t, a, ref)
	releaseOld()

	// Hold the first successful reload after its read and join its completion.
	reached, resume, completed := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	var resumeOnce sync.Once
	resumeLoad := func() { resumeOnce.Do(func() { close(resume) }) }
	defer resumeLoad()
	testHookStateLoaded.Store(new(func(objectID string, complete func()) {
		held := false
		if objectID == id {
			once.Do(func() { held = true })
		}
		if !held {
			complete()
			return
		}
		close(reached)
		<-resume
		complete()
		close(completed)
	}))
	defer testHookStateLoaded.Store(nil)
	replaceTestState(ctx, t, a, id, old.GetPrivKey(), []byte("held"))
	select {
	case <-reached:
	case <-ctx.Done():
		t.Fatal("replacement did not load the state")
	}

	// The delete supersedes the middle restart while it waits for the held load.
	replaceTestState(ctx, t, a, id, old.GetPrivKey(), []byte("middle"))
	if err := a.DeleteSharedObject(ctx, id); err != nil {
		t.Fatal(err)
	}
	expectTestMountNotFound(ctx, t, a, ref)
	resumeLoad()
	select {
	case <-completed:
	case <-ctx.Done():
		t.Fatal("retired load did not finish")
	}

	// Use the real enrollment placeholder path, keeping the refusal retained.
	if err := a.writeGenesisSOState(ctx, id); err != nil {
		t.Fatal(err)
	}
	fresh, releaseFresh, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatalf("mount after placeholder commit with health retained: %v", err)
	}
	defer releaseFresh()
	if got := readTestStateData(ctx, t, fresh.(*SharedObject)); len(got) != 0 {
		t.Fatalf("enrollment placeholder took state from a retired run: %q", got)
	}
}

// TestRecreatedMountSurvivesCanceledStart checks that a delayed tracker
// callback canceled by a restart cannot clear the recreated object's mount.
func TestRecreatedMountSurvivesCanceledStart(t *testing.T) {
	// Keep the tracker live across the delete and recreate.
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	_, _, a, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()
	const id = "late-start"
	ref := createTestSharedObject(ctx, t, a, id)
	_, releaseOld := mountTestSharedObject(ctx, t, a, ref)
	defer releaseOld()
	if err := a.DeleteSharedObject(ctx, id); err != nil {
		t.Fatal(err)
	}
	createTestSharedObject(ctx, t, a, id)
	fresh, releaseFresh := mountTestSharedObject(ctx, t, a, ref)
	defer releaseFresh()

	// Run the entry point as if an older keyed callback reached it only
	// after cancellation and the newer run had already published.
	staleCtx, cancelStale := context.WithCancel(ctx)
	cancelStale()
	if err := fresh.tkr.executeSharedObjectTracker(staleCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled tracker callback returned %v", err)
	}

	// A new waiter must get the current handle and its write must survive.
	mountCtx, cancelMount := context.WithTimeout(ctx, time.Second)
	defer cancelMount()
	mounted, releaseMounted := mountTestSharedObject(mountCtx, t, a, ref)
	defer releaseMounted()
	if mounted != fresh {
		t.Fatal("a canceled start replaced the current mount")
	}
	writeTestStateData(ctx, t, mounted, []byte("after-late-start"))
}

// TestCanceledMountStartCannotClaimAfterLockWait makes cancellation observable
// only after a live start finds the result lock held. Moving the check before
// the lock would let the canceled start replace the published result.
func TestCanceledMountStartCannotClaimAfterLockWait(t *testing.T) {
	// Publish one result, then hold its lock while starting the other callback.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	res := newSobjectResult()
	live, err := res.start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ready := newTestRunSharedObject()
	health := sobject.NewSharedObjectReadyHealth(sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT)
	live.publish(health, ready, nil)

	// Hold the real result lock and let the failed acquisition signal when
	// the live callback has reached it. Keep the callback there until canceled.
	waiting, resume := make(chan struct{}), make(chan struct{})
	res.testClaimWait = func() {
		close(waiting)
		<-resume
	}
	result := make(chan error, 1)
	res.mtx.Lock()
	go func() {
		_, err := res.start(ctx)
		result <- err
	}()

	// Cancel after contention is observed but before releasing the mutex.
	select {
	case <-waiting:
	case <-time.After(5 * time.Second):
		cancel()
		close(resume)
		res.mtx.Unlock()
		t.Fatal("mount claim did not reach the held result lock")
	}
	cancel()
	close(resume)
	res.mtx.Unlock()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Errorf("mount claim returned %v after cancellation under contention", err)
	}

	// A rejected claim changes neither generation nor the ready publication.
	waitCtx, cancelWait := context.WithTimeout(t.Context(), time.Second)
	defer cancelWait()
	got, err := res.await(waitCtx)
	if err != nil || got != ready || res.gen != live.gen || res.watchHealth().GetValue() != health {
		t.Fatalf("canceled claim changed the current generation, result or health: mount error %v", err)
	}
}

// TestRetiredRunWritesNothing checks that a run a retire or a later run has
// replaced cannot publish, change health or clear the result, even while its
// context is live.
func TestRetiredRunWritesNothing(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	layer := sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT
	ready := sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_READY
	res := newSobjectResult()

	// A retired run's publish is dropped.
	old, err := res.start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	res.retire(false)
	old.publish(sobject.NewSharedObjectReadyHealth(layer), newTestRunSharedObject(), nil)
	expectTestResultCleared(ctx, t, &sobjectTracker{res: res})
	if res.watchHealth().GetValue().GetStatus() == ready {
		t.Fatal("a retired run published its health")
	}

	// The newest run publishes, and the retired run cannot change the
	// newest run's health or clear the newest run's result.
	run, err := res.start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := newTestRunSharedObject()
	run.publish(sobject.NewSharedObjectReadyHealth(layer), want, nil)
	old.swapHealth(func(*sobject.SharedObjectHealth) *sobject.SharedObjectHealth {
		return sobject.NewSharedObjectLoadingHealth(layer)
	})
	old.end()
	if res.watchHealth().GetValue().GetStatus() != ready {
		t.Fatal("a retired run changed the health")
	}
	got, err := res.await(ctx)
	if err != nil || got != want {
		t.Fatalf("the newest run's result is %v, %v after the retired run wrote", got, err)
	}

	// A retire clears the result and reports loading, so a watcher does not
	// see the retired run's ready health before the next run starts.
	res.retire(false)
	expectTestResultCleared(ctx, t, &sobjectTracker{res: res})
	if res.watchHealth().GetValue().GetStatus() != sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_LOADING {
		t.Fatal("a retire left the retired run's health")
	}
	if !want.lsoHost.ended {
		t.Fatal("a retire left the retired run's state snapshot published")
	}
}

// TestMountListedObjectWithoutState lists an object with no stored state and a
// stray local-state key, as an old create did, and checks that its mount
// stores a genesis without the stray key, while an unlisted id stays refused.
func TestMountListedObjectWithoutState(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	_, _, a, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()

	// Leave a stray key, then list the object as the old create did. The
	// account's P2P sync may mount it as soon as it is listed.
	putTestStrayKey(ctx, t, a, "legacy")
	ref := listTestSharedObjectWithoutState(ctx, t, a, "legacy")

	// The listed object mounts with an empty genesis and no stray key.
	createTestSharedObject(ctx, t, a, "fresh")
	so, rel := mountTestSharedObject(ctx, t, a, ref)
	defer rel()
	if got := readTestStateData(ctx, t, so); len(got) != 0 {
		t.Fatalf("the listed object reads %q", got)
	}
	want := countTestKeys(ctx, t, a, SobjectObjectStorePrefix("fresh"))
	if n := countTestKeys(ctx, t, a, SobjectObjectStorePrefix("legacy")); n != want {
		t.Fatalf("the listed object holds %d keys, want %d", n, want)
	}

	// An unlisted id with no state is refused.
	providerID := a.t.accountInfo.GetProviderId()
	accountID := a.t.accountInfo.GetProviderAccountId()
	expectTestMountNotFound(ctx, t, a, sobject.NewSharedObjectRef(providerID, accountID, "unlisted", SobjectBlockStoreID("unlisted")))

	// An object unlisted between the mount's read and its genesis write is
	// refused by the write's reread of the list, and gets no state. The hook
	// is set before the listing, so it catches whichever mount comes first.
	testHookStateListed.Store(new(func(id string) {
		if id != "unlisting" {
			return
		}
		list := a.soListCtr.GetValue().CloneVT()
		list.SharedObjects = slices.DeleteFunc(list.SharedObjects, func(e *sobject.SharedObjectListEntry) bool {
			return e.GetRef().GetProviderResourceRef().GetId() == id
		})
		if err := a.writeSharedObjectList(ctx, list); err != nil {
			t.Error(err)
		}
		a.soListCtr.SetValue(list)
	}))
	t.Cleanup(func() { testHookStateListed.Store(nil) })
	ref = listTestSharedObjectWithoutState(ctx, t, a, "unlisting")
	expectTestMountNotFound(ctx, t, a, ref)
	if n := countTestKeys(ctx, t, a, SobjectObjectStorePrefix("unlisting")); n != 0 {
		t.Fatalf("the object unlisted before its genesis write holds %d keys", n)
	}
}

// TestArrivingObjectMountsOnceWritten checks that an object a transfer lists
// as arriving refuses mounts with ErrSharedObjectArriving until the
// transfer's write, which a write expecting another state cannot make, and
// that a delete removes an arriving object so a create of its id starts
// fresh.
func TestArrivingObjectMountsOnceWritten(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	_, _, a, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()

	// Write as the account's volume peer, which a transfer signs with.
	peer, err := a.vol.GetPeer(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := peer.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	meta := &sobject.SharedObjectMeta{BodyType: "space"}
	mount := func(id string) (*SharedObject, func(), error) {
		mctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		ref := sobject.NewSharedObjectRef(a.t.accountInfo.GetProviderId(), a.t.accountInfo.GetProviderAccountId(), id, SobjectBlockStoreID(id))
		so, rel, err := a.MountSharedObject(mctx, ref, nil)
		if err != nil {
			return nil, nil, err
		}
		return so.(*SharedObject), rel, nil
	}

	// An arriving object refuses mounts.
	if err := a.CreateArrivingSharedObject(ctx, "arriving", meta, []byte("source")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := mount("arriving"); !errors.Is(err, sobject.ErrSharedObjectArriving) {
		t.Fatalf("a mount of an arriving object returned %v", err)
	}

	// A write expecting another state writes nothing. The write expecting
	// the arriving state lets it mount with the written state.
	stored, err := a.ReadStoredSharedObject(ctx, "arriving")
	if err != nil || !stored.Origin.GetArriving() {
		t.Fatalf("read arriving object: %v %v", stored, err)
	}
	stale := ReplaceOrigin{Generation: stored.Entry.GetGeneration(), State: []byte("another state"), Source: []byte("source")}
	if err := a.ReplaceSharedObjectState(ctx, a.le, "arriving", owner, []byte("moved"), stale); !errors.Is(err, ErrStateChanged) {
		t.Fatalf("a write expecting another state returned %v", err)
	}
	expect := ReplaceOrigin{Generation: stored.Entry.GetGeneration(), State: stored.State, Source: []byte("source")}
	if err := a.ReplaceSharedObjectState(ctx, a.le, "arriving", owner, []byte("moved"), expect); err != nil {
		t.Fatal(err)
	}
	so, rel, err := mount("arriving")
	if err != nil {
		t.Fatal(err)
	}
	if got := readTestStateData(ctx, t, so); !bytes.Equal(got, []byte("moved")) {
		t.Fatalf("the written object reads %q", got)
	}
	rel()
	if written, err := a.ReadStoredSharedObject(ctx, "arriving"); err != nil || written.Origin.GetArriving() || !written.Unchanged() {
		t.Fatalf("written origin = %v, %v; want written and unchanged", written.Origin, err)
	}

	// A delete removes an arriving object, and a create of its id mounts.
	if err := a.CreateArrivingSharedObject(ctx, "dropped", meta, []byte("source")); err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteSharedObject(ctx, "dropped"); err != nil {
		t.Fatal(err)
	}
	if n := countTestKeys(ctx, t, a, SobjectObjectStorePrefix("dropped")); n != 0 {
		t.Fatalf("the deleted arriving object left %d keys", n)
	}
	createTestSharedObject(ctx, t, a, "dropped")
	_, rel, err = mount("dropped")
	if err != nil {
		t.Fatalf("a create of a deleted arriving id does not mount: %v", err)
	}
	rel()
}
