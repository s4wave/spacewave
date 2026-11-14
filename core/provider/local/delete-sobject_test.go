package provider_local

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/crypto"
)

// TestDeleteSharedObjectRemovesOnlyItsState deletes an object whose id is a
// prefix of another's, holding more keys than one delete batch, and checks
// that only the named object's keys go, that a handle taken before the delete
// cannot write it back, and that a later mount is refused rather than given a
// fresh genesis.
func TestDeleteSharedObjectRemovesOnlyItsState(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	_, _, a, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()

	// Create two objects whose ids share a prefix and give each a payload.
	deleted := createTestSharedObject(ctx, t, a, "so1")
	kept := createTestSharedObject(ctx, t, a, "so1x")
	stale, releaseStale := mountTestSharedObject(ctx, t, a, deleted)
	defer releaseStale()
	keptSO, releaseKept := mountTestSharedObject(ctx, t, a, kept)
	writeTestStateData(ctx, t, stale, []byte("fake-deleted"))
	writeTestStateData(ctx, t, keptSO, []byte("fake-kept"))

	// Give the first object more keys than one delete batch holds.
	if err := tryWriteTestLocalKeys(ctx, stale, 3*deleteBatchSize); err != nil {
		t.Fatal(err)
	}
	releaseKept()
	keptKeys := countTestKeys(ctx, t, a, SobjectObjectStorePrefix("so1x"))

	// Hold the state watchable and a local state store across the delete.
	stateCtr, relStateCtr, err := stale.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relStateCtr()
	localStore, relLocalStore, err := stale.AccessLocalStateStore(ctx, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relLocalStore()

	// Delete the first object.
	if err := a.DeleteSharedObject(ctx, "so1"); err != nil {
		t.Fatal(err)
	}

	// Neither the watchable nor the local store taken before the delete
	// serves the deleted state once it returns.
	if snap := stateCtr.GetValue(); snap != nil {
		t.Fatal("a state watchable taken before the delete still holds its snapshot")
	}
	if tx, err := localStore.NewTransaction(ctx, false); err == nil {
		tx.Discard()
		t.Fatal("a local store taken before the delete still reads")
	}

	// Only the first object's keys and list entry are gone.
	if n := countTestKeys(ctx, t, a, SobjectObjectStorePrefix("so1")); n != 0 {
		t.Fatalf("delete left %d keys under so/so1/", n)
	}
	if n := countTestKeys(ctx, t, a, SobjectObjectStorePrefix("so1x")); n != keptKeys {
		t.Fatalf("delete changed the other object's keys: %d, want %d", n, keptKeys)
	}
	if testListed(a, "so1") || !testListed(a, "so1x") {
		t.Fatal("delete did not unlist exactly the named object")
	}

	// The handle taken before the delete cannot write the state back.
	if err := tryWriteTestStateData(ctx, stale, []byte("fake-resurrected")); err == nil {
		t.Fatal("a handle taken before the delete wrote its state")
	}
	if n := countTestKeys(ctx, t, a, SobjectObjectStorePrefix("so1")); n != 0 {
		t.Fatalf("a stale write left %d keys under so/so1/", n)
	}

	// A mount of the deleted object is refused, and the other still reads.
	expectTestMountNotFound(ctx, t, a, deleted)
	keptSO, releaseKept = mountTestSharedObject(ctx, t, a, kept)
	defer releaseKept()
	if got := readTestStateData(ctx, t, keptSO); !bytes.Equal(got, []byte("fake-kept")) {
		t.Fatalf("the other object reads %q after the delete", got)
	}
}

// TestDeleteSharedObjectRelistStartsFresh deletes an object, creates one with
// the same id, and checks that the new object carries none of the old state
// and that a handle taken before the delete cannot write over it.
func TestDeleteSharedObjectRelistStartsFresh(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	_, _, a, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()

	// Write a payload, delete the object and create it again.
	ref := createTestSharedObject(ctx, t, a, "relist")
	stale, releaseStale := mountTestSharedObject(ctx, t, a, ref)
	defer releaseStale()
	writeTestStateData(ctx, t, stale, []byte("fake-old"))
	if err := a.DeleteSharedObject(ctx, "relist"); err != nil {
		t.Fatal(err)
	}
	ref = createTestSharedObject(ctx, t, a, "relist")

	// The relisted object mounts with an empty genesis, then takes its own
	// payload.
	fresh, releaseFresh := mountTestSharedObject(ctx, t, a, ref)
	defer releaseFresh()
	if got := readTestStateData(ctx, t, fresh); len(got) != 0 {
		t.Fatalf("the relisted object reads %q", got)
	}
	writeTestStateData(ctx, t, fresh, []byte("fake-new"))
	keys := countTestKeys(ctx, t, a, SobjectObjectStorePrefix("relist"))

	// The stale handle writes neither state nor local state: the delete ended
	// its mount, so both refuse on the ended-mount guard before reading the
	// relisted object's host key.
	if err := tryWriteTestStateData(ctx, stale, []byte("fake-stale")); !errors.Is(err, context.Canceled) {
		t.Fatalf("a stale state write returned %v, want the ended-mount refusal", err)
	}
	if err := tryWriteTestLocalKeys(ctx, stale, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("a stale local-state write returned %v, want the ended-mount refusal", err)
	}

	// The relisted object's state and keys are unchanged.
	if got := readTestStateData(ctx, t, fresh); !bytes.Equal(got, []byte("fake-new")) {
		t.Fatalf("the relisted object reads %q after the stale writes", got)
	}
	if n := countTestKeys(ctx, t, a, SobjectObjectStorePrefix("relist")); n != keys {
		t.Fatalf("the stale writes changed the relisted object's keys: %d, want %d", n, keys)
	}
}

// TestDeleteSharedObjectRacingHandleWriteFails holds a delete between its
// commit and the restart of the live mount, and checks that the still-running
// handle can write neither state nor local state there.
func TestDeleteSharedObjectRacingHandleWriteFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	_, _, a, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()

	// Mount the object and write through the handle inside the delete.
	ref := createTestSharedObject(ctx, t, a, "racing")
	so, rel := mountTestSharedObject(ctx, t, a, ref)
	defer rel()
	var stateErr, localErr error
	var ranHook bool
	testHookDeleteCommitted = func(id string) {
		if id != "racing" {
			return
		}
		ranHook = true
		if so.ctx.Err() != nil {
			t.Error("the mount ended before the delete restarted it")
		}
		stateErr = tryWriteTestStateData(ctx, so, []byte("fake-racing"))
		localErr = tryWriteTestLocalKeys(ctx, so, 1)
	}
	t.Cleanup(func() { testHookDeleteCommitted = nil })
	if err := a.DeleteSharedObject(ctx, "racing"); err != nil {
		t.Fatal(err)
	}

	// Both writes refused on the host-key guard, and nothing survives.
	if !ranHook {
		t.Fatal("the delete did not reach its commit hook")
	}
	if !errors.Is(stateErr, sobject.ErrSharedObjectNotFound) {
		t.Fatalf("a racing state write returned %v, want the host-key refusal", stateErr)
	}
	if !errors.Is(localErr, sobject.ErrSharedObjectNotFound) {
		t.Fatalf("a racing local-state write returned %v, want the host-key refusal", localErr)
	}
	if n := countTestKeys(ctx, t, a, SobjectObjectStorePrefix("racing")); n != 0 {
		t.Fatalf("delete left %d keys under so/racing/", n)
	}
	expectTestMountNotFound(ctx, t, a, ref)
}

// TestRefusedMountRevivesOnCreate holds a mount between reading that its
// object has no state and publishing the refusal, creates the object there,
// and checks that the tracker restarts and mounts the new object while the
// first reference still holds it.
func TestRefusedMountRevivesOnCreate(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	_, _, a, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()

	// Hold the first refusal of the unlisted id.
	reached, resume := make(chan struct{}), make(chan struct{})
	var once sync.Once
	testHookStateRefused = func(id string) {
		if id != "revive" {
			return
		}
		once.Do(func() {
			close(reached)
			<-resume
		})
	}
	t.Cleanup(func() { testHookStateRefused = nil })

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
	testHookRunEnding = func(id string, end func()) {
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
	}
	t.Cleanup(func() { testHookRunEnding = nil })
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
	if err := a.ReplaceSharedObjectState(ctx, a.le, "chain", owner, []byte("fake-transfer")); err != nil {
		t.Fatal(err)
	}
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
	old := res.start(t.Context())
	res.retire()
	old.publish(sobject.NewSharedObjectReadyHealth(layer), &SharedObject{}, nil)
	expectTestResultCleared(ctx, t, &sobjectTracker{res: res})
	if res.watchHealth().GetValue().GetStatus() == ready {
		t.Fatal("a retired run published its health")
	}

	// The newest run publishes, and the retired run cannot change or clear it.
	run := res.start(t.Context())
	want := &SharedObject{}
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
	res.retire()
	expectTestResultCleared(ctx, t, &sobjectTracker{res: res})
	if res.watchHealth().GetValue().GetStatus() != sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_LOADING {
		t.Fatal("a retire left the retired run's health")
	}
}

// TestDeleteSharedObjectInterruptedCompletesOnRerun stops a delete between its
// two steps, after it publishes the catalog entry and before the local delete
// commits, and checks that the object stays listed with all its state and
// that a rerun finishes the delete.
func TestDeleteSharedObjectInterruptedCompletesOnRerun(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	_, _, a, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()

	// Give the object state.
	ref := createTestSharedObject(ctx, t, a, "rerun")
	so, rel := mountTestSharedObject(ctx, t, a, ref)
	writeTestStateData(ctx, t, so, []byte("fake-rerun"))
	rel()
	keys := countTestKeys(ctx, t, a, SobjectObjectStorePrefix("rerun"))

	// Hold mtx so catalog replay cannot finish the delete, and end the
	// delete's context once its catalog entry is published.
	relMtx, err := a.mtx.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dctx, dcancel := context.WithCancel(ctx)
	defer dcancel()
	var published bool
	testHookDeletePublished = func(id string) {
		if id == "rerun" {
			published = true
			dcancel()
		}
	}
	t.Cleanup(func() { testHookDeletePublished = nil })
	err = a.DeleteSharedObject(dctx, "rerun")
	testHookDeletePublished = nil

	// The delete stopped after its publish, and the local state is whole.
	if !published || !errors.Is(err, context.Canceled) {
		relMtx()
		t.Fatalf("the interrupted delete returned %v after publishing: %v", err, published)
	}
	listed := testListed(a, "rerun")
	n := countTestKeys(ctx, t, a, SobjectObjectStorePrefix("rerun"))
	relMtx()
	if !listed {
		t.Fatal("the interrupted delete unlisted the object")
	}
	if n != keys {
		t.Fatalf("the interrupted delete left %d keys, want %d", n, keys)
	}

	// A rerun removes the list entry and the state. Catalog replay may finish
	// the delete first, and then the rerun finds nothing to delete.
	if err := a.DeleteSharedObject(ctx, "rerun"); err != nil && !errors.Is(err, sobject.ErrSharedObjectNotFound) {
		t.Fatal(err)
	}
	if testListed(a, "rerun") {
		t.Fatal("the rerun left the object listed")
	}
	if n := countTestKeys(ctx, t, a, SobjectObjectStorePrefix("rerun")); n != 0 {
		t.Fatalf("the rerun left %d keys under so/rerun/", n)
	}
}

// TestDeleteStateOffKeepsOldBehavior selects the old behavior and checks that
// delete keeps the stored state, create of the same id adopts it, and a mount
// of an unlisted id with no state writes a genesis.
func TestDeleteStateOffKeepsOldBehavior(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	_, _, a, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()
	setTestDeleteStateOff(t, true)

	// Give an object state and delete it: the state stays.
	ref := createTestSharedObject(ctx, t, a, "oldway")
	so, rel := mountTestSharedObject(ctx, t, a, ref)
	writeTestStateData(ctx, t, so, []byte("fake-kept"))
	rel()
	keys := countTestKeys(ctx, t, a, SobjectObjectStorePrefix("oldway"))
	if err := a.DeleteSharedObject(ctx, "oldway"); err != nil {
		t.Fatal(err)
	}
	if testListed(a, "oldway") {
		t.Fatal("delete left the object listed")
	}
	if n := countTestKeys(ctx, t, a, SobjectObjectStorePrefix("oldway")); n != keys {
		t.Fatalf("delete under the old behavior left %d keys, want %d", n, keys)
	}

	// Create of the same id writes only the list and adopts the kept state.
	ref = createTestSharedObject(ctx, t, a, "oldway")
	so, rel = mountTestSharedObject(ctx, t, a, ref)
	if got := readTestStateData(ctx, t, so); !bytes.Equal(got, []byte("fake-kept")) {
		t.Fatalf("the recreated object reads %q", got)
	}
	rel()

	// A mount of an unlisted id with no state writes a genesis.
	providerID := a.t.accountInfo.GetProviderId()
	accountID := a.t.accountInfo.GetProviderAccountId()
	ghost := sobject.NewSharedObjectRef(providerID, accountID, "unlisted", SobjectBlockStoreID("unlisted"))
	_, rel = mountTestSharedObject(ctx, t, a, ghost)
	rel()
}

// TestCreateSharedObjectStaleKeys checks what a create clears under an
// unlisted id: state no enrollment is writing, left by an old delete, an off
// mode delete or an abandoned enrollment, goes, and the new lifetime reads
// only its genesis; state an enrollment is writing stays and fails the
// create.
func TestCreateSharedObjectStaleKeys(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	_, _, a, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()
	createTestSharedObject(ctx, t, a, "fresh")
	want := countTestKeys(ctx, t, a, SobjectObjectStorePrefix("fresh"))

	// expectFresh creates id and checks it holds and reads only a genesis.
	expectFresh := func(t *testing.T, id string) {
		ref := createTestSharedObject(ctx, t, a, id)
		if n := countTestKeys(ctx, t, a, SobjectObjectStorePrefix(id)); n != want {
			t.Fatalf("the created object holds %d keys, want %d", n, want)
		}
		so, rel := mountTestSharedObject(ctx, t, a, ref)
		defer rel()
		if got := readTestStateData(ctx, t, so); len(got) != 0 {
			t.Fatalf("the created object reads %q", got)
		}
	}

	// leaveState writes state and n local-state keys under id, then deletes
	// it with the state delete off, which keeps them, as released versions
	// did.
	leaveState := func(t *testing.T, id string, n int) {
		setTestDeleteStateOff(t, true)
		defer setTestDeleteStateOff(t, false)
		ref := createTestSharedObject(ctx, t, a, id)
		so, rel := mountTestSharedObject(ctx, t, a, ref)
		writeTestStateData(ctx, t, so, []byte("fake-stale"))
		if err := tryWriteTestLocalKeys(ctx, so, n); err != nil {
			t.Fatal(err)
		}
		rel()
		if err := a.DeleteSharedObject(ctx, id); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("old-version-leftover", func(t *testing.T) {
		// More keys than one batch, as a long lived object leaves.
		leaveState(t, "old", deleteBatchSize+10)
		expectFresh(t, "old")
	})

	t.Run("off-mode-leftover", func(t *testing.T) {
		leaveState(t, "stale", 1)
		expectFresh(t, "stale")
	})

	// leaveEnrolled writes a placeholder for id and holds a root in its
	// bucket, as an import does before it lists the id.
	leaveEnrolled := func(t *testing.T, id string) {
		if err := a.writeGenesisSOState(ctx, id); err != nil {
			t.Fatal(err)
		}
		ref := sobject.NewSharedObjectRef(a.GetProviderID(), a.GetAccountID(), id, SobjectBlockStoreID(id))
		so, rel := mountTestSharedObject(ctx, t, a, ref)
		putRehearsalTree(ctx, t, so, id, "rehearsal")
		rel()
		requireRehearsalRoot(ctx, t, a, id, true)
	}

	t.Run("abandoned-placeholder", func(t *testing.T) {
		// An enrollment that returned without listing the id left it and
		// its hold, which the create releases.
		leaveEnrolled(t, "abandoned")
		expectFresh(t, "abandoned")
		requireRehearsalRoot(ctx, t, a, "abandoned", false)
	})

	t.Run("abandoned-placeholder-off-mode", func(t *testing.T) {
		// The old behavior keeps the hold.
		leaveEnrolled(t, "abandoned-off")
		setTestDeleteStateOff(t, true)
		defer setTestDeleteStateOff(t, false)
		createTestSharedObject(ctx, t, a, "abandoned-off")
		requireRehearsalRoot(ctx, t, a, "abandoned-off", true)
	})

	t.Run("enrollment-in-flight", func(t *testing.T) {
		// A registered enrollment's placeholder stays and fails the create.
		relMtx, err := a.mtx.Lock(ctx)
		if err != nil {
			t.Fatal(err)
		}
		end := a.beginEnrollmentLocked("enrolling")
		relMtx()
		leaveEnrolled(t, "enrolling")
		_, err = a.CreateSharedObject(ctx, "enrolling", &sobject.SharedObjectMeta{BodyType: "test"}, "", "")
		if !errors.Is(err, errSOEnrolling) {
			t.Fatalf("a create during an enrollment returned %v", err)
		}
		requireRehearsalRoot(ctx, t, a, "enrolling", true)
		if n := countTestKeys(ctx, t, a, SobjectObjectStorePrefix("enrolling")); n != want {
			t.Fatalf("the refused create left %d keys, want %d", n, want)
		}
		if testListed(a, "enrolling") {
			t.Fatal("the refused create listed the id")
		}

		// Once the enrollment returns unlisted, the placeholder is
		// abandoned.
		end()
		expectFresh(t, "enrolling")
		requireRehearsalRoot(ctx, t, a, "enrolling", false)
	})

	t.Run("no-state", func(t *testing.T) {
		// Keys with no state beside them belong to no object: the mount
		// path's clear removes them.
		putTestStrayKey(ctx, t, a, "stray")
		if err := a.clearStaleSOKeys(ctx, "stray"); err != nil {
			t.Fatal(err)
		}
		if n := countTestKeys(ctx, t, a, SobjectObjectStorePrefix("stray")); n != 0 {
			t.Fatalf("the clear left %d keys", n)
		}
		putTestStrayKey(ctx, t, a, "stray")
		expectFresh(t, "stray")
	})

	t.Run("state-kept-on-mount-path", func(t *testing.T) {
		// The mount path's clear keeps keys beside stored state.
		leaveState(t, "kept", 1)
		n := countTestKeys(ctx, t, a, SobjectObjectStorePrefix("kept"))
		if err := a.clearStaleSOKeys(ctx, "kept"); err != nil {
			t.Fatal(err)
		}
		if got := countTestKeys(ctx, t, a, SobjectObjectStorePrefix("kept")); got != n {
			t.Fatalf("the clear left %d of %d keys", got, n)
		}
	})

	t.Run("keys-reappear", func(t *testing.T) {
		// Keys that appear between the clear and the write are cleared and
		// the write retried once; keys that keep appearing fail it.
		for _, always := range []bool{false, true} {
			var calls int
			err := a.withClearedSOKeys(ctx, "reappear", a.clearStaleSOKeys, func() error {
				calls++
				if calls == 1 || always {
					putTestStrayKey(ctx, t, a, "reappear")
				}
				return a.runSoObjectStoreTx(ctx, false, refuseStaleSOKeys("reappear"))
			})
			if calls != 2 {
				t.Fatalf("the write ran %d times, want 2", calls)
			}
			if always != errors.Is(err, errStaleSOKeys) {
				t.Fatalf("with keys always appearing %v, the write returned %v", always, err)
			}
		}
	})
}

// listTestSharedObjectWithoutState lists id with a block store and no stored
// state, as a create before creates stored a genesis did, without flipping
// DeleteStateEnv, which background mounts read.
func listTestSharedObjectWithoutState(ctx context.Context, t *testing.T, a *ProviderAccount, id string) *sobject.SharedObjectRef {
	relMtx, err := a.mtx.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer relMtx()
	ref := sobject.NewSharedObjectRef(a.t.accountInfo.GetProviderId(), a.t.accountInfo.GetProviderAccountId(), id, SobjectBlockStoreID(id))
	if _, err := a.createBlockStoreLocked(ctx, SobjectBlockStoreID(id)); err != nil {
		t.Fatal(err)
	}
	list := a.soListCtr.GetValue().CloneVT()
	insertListEntry(list, newListEntry(ref, &sobject.SharedObjectMeta{BodyType: "test"}, "", nil))
	if err := a.writeSharedObjectList(ctx, list); err != nil {
		t.Fatal(err)
	}
	a.soListCtr.SetValue(list)
	return ref
}

// putTestStrayKey stores a local-state key under id with no state beside it.
func putTestStrayKey(ctx context.Context, t *testing.T, a *ProviderAccount, id string) {
	key := append(SobjectObjectStorePrefix(id), "ls/fake-store/fake-key"...)
	err := a.runSoObjectStoreTx(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
		return tx.Set(ctx, key, []byte("fake-value"))
	})
	if err != nil {
		t.Fatal(err)
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
	testHookStateListed = func(id string) {
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
	}
	t.Cleanup(func() { testHookStateListed = nil })
	ref = listTestSharedObjectWithoutState(ctx, t, a, "unlisting")
	expectTestMountNotFound(ctx, t, a, ref)
	if n := countTestKeys(ctx, t, a, SobjectObjectStorePrefix("unlisting")); n != 0 {
		t.Fatalf("the object unlisted before its genesis write holds %d keys", n)
	}
}

// TestStateWritesRefuseUnownedIds checks that a transfer write to an unlisted
// id and an enrollment with an invalid id write nothing.
func TestStateWritesRefuseUnownedIds(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	_, _, a, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()

	// A transfer write to an unlisted id fails and stores nothing.
	owner, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	err = a.ReplaceSharedObjectState(ctx, a.le, "unlisted", owner, []byte("fake-world"))
	if !errors.Is(err, sobject.ErrSharedObjectNotFound) {
		t.Fatalf("a transfer write to an unlisted id returned %v", err)
	}
	if n := countTestKeys(ctx, t, a, SobjectObjectStorePrefix("unlisted")); n != 0 {
		t.Fatalf("a transfer write to an unlisted id left %d keys", n)
	}

	// An enrollment with an id that is not a DNS label fails before writing.
	// Its ref check is the only one before the placeholder write: the genesis
	// builder does not validate the id, so without it keys land under so/a/.
	_, err = a.mountEnrolledSO(ctx, "a/x", &sobject.SharedObjectMeta{BodyType: "space"}, "shared", nil, &sobject.SOState{}, nil, nil)
	if err == nil {
		t.Fatal("an enrollment with an invalid id succeeded")
	}
	if n := countTestKeys(ctx, t, a, []byte("so/a/")); n != 0 {
		t.Fatalf("an enrollment with an invalid id left %d keys", n)
	}
}

// TestDeleteSharedObjectRefusesSettings checks that the bound settings object
// cannot be deleted.
func TestDeleteSharedObjectRefusesSettings(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	_, _, a, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()

	// Delete the settings object and check it stays listed with its state.
	settings, err := a.GetAccountSettingsRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id := settings.GetProviderResourceRef().GetId()
	if err := a.DeleteSharedObject(ctx, id); err == nil {
		t.Fatal("delete of the settings object succeeded")
	}

	// The locked path that catalog replay and settings cleanup use refuses
	// it as well.
	relMtx, err := a.mtx.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = a.deleteSharedObjectLocked(ctx, id)
	relMtx()
	if !errors.Is(err, errCannotDeleteSettings) {
		t.Fatalf("locked delete of the settings object returned %v", err)
	}
	if !testListed(a, id) {
		t.Fatal("the settings object was unlisted")
	}
	if n := countTestKeys(ctx, t, a, SobjectObjectStorePrefix(id)); n == 0 {
		t.Fatal("the settings object lost its state")
	}
}

// holdTestRunStart holds the next run of id before it takes its handle until
// the returned function is called.
func holdTestRunStart(t *testing.T, id string) func() {
	resume := make(chan struct{})
	var held, released sync.Once
	testHookRunStarting = func(hid string) {
		if hid != id {
			return
		}
		hold := false
		held.Do(func() { hold = true })
		if hold {
			<-resume
		}
	}
	release := func() { released.Do(func() { close(resume) }) }
	t.Cleanup(func() {
		release()
		testHookRunStarting = nil
	})
	return release
}

// expectTestResultCleared checks that the tracker holds no mount result: a
// set result returns at once, so a wait that reaches its deadline shows none.
// The await returns context.Canceled, not the context's own error, when its
// context ends.
func expectTestResultCleared(ctx context.Context, t *testing.T, tkr *sobjectTracker) {
	t.Helper()
	wctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	so, err := tkr.res.await(wctx)
	if !errors.Is(err, context.Canceled) || wctx.Err() == nil {
		t.Fatalf("the tracker holds a result: %v, %v", so != nil, err)
	}
}

// TestReconcileKeepsRecreatedObject deletes an object, creates one under the
// same id without publishing it, as a crash between the create's commit and
// its catalog publish leaves it, writes to it, and runs the catalog reconcile
// before and after the republish, as a restart would. The newer object and
// its acknowledged write survive the older object's tombstone. An object
// without a generation is still removed by its tombstone, which has none.
func TestReconcileKeepsRecreatedObject(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	_, _, a, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()
	settingsRef, err := a.GetAccountSettingsRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settingsID := settingsRef.GetProviderResourceRef().GetId()
	reconcile := func() {
		t.Helper()
		settings, err := a.readAccountSettings(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := a.reconcileAccountCatalog(ctx, settingsID, settings, a.soListCtr.GetValue()); err != nil {
			t.Fatal(err)
		}
	}

	// Delete the first object, leaving its tombstone in the catalog.
	ref := createTestSharedObject(ctx, t, a, "reuse")
	if err := a.DeleteSharedObject(ctx, "reuse"); err != nil {
		t.Fatal(err)
	}

	// Create the id again and commit it without the catalog publish.
	relMtx, err := a.mtx.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.createSharedObjectLocked(ctx, "reuse", &sobject.SharedObjectMeta{BodyType: "space"})
	relMtx()
	if err != nil {
		t.Fatal(err)
	}
	fresh, releaseFresh := mountTestSharedObject(ctx, t, a, ref)
	writeTestStateData(ctx, t, fresh, []byte("fake-new"))
	releaseFresh()

	// The reconcile republishes the newer object over the tombstone, and a
	// later reconcile, as after a restart, finds it live.
	reconcile()
	reconcile()
	settings, err := a.readAccountSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if entry := settings.FindCatalogEntry("reuse"); entry == nil || entry.GetDeleted() {
		t.Fatal("the reconcile left the recreated object tombstoned in the catalog")
	}
	if !testListed(a, "reuse") {
		t.Fatal("the reconcile removed the recreated object")
	}
	fresh, releaseFresh = mountTestSharedObject(ctx, t, a, ref)
	defer releaseFresh()
	if got := readTestStateData(ctx, t, fresh); !bytes.Equal(got, []byte("fake-new")) {
		t.Fatalf("the recreated object reads %q after the reconcile", got)
	}

	// An object listed and cataloged without a generation, as before
	// generations, is still removed by its tombstone, also without one.
	legacyRef := createTestSharedObject(ctx, t, a, "legacy")
	relMtx, err = a.mtx.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	list := a.soListCtr.GetValue().CloneVT()
	var legacy *sobject.SharedObjectListEntry
	for _, entry := range list.GetSharedObjects() {
		if entry.GetRef().EqualVT(legacyRef) {
			entry.Generation = ""
			legacy = entry.CloneVT()
		}
	}
	err = a.writeSharedObjectList(ctx, list)
	if err == nil {
		a.soListCtr.SetValue(list)
	}
	relMtx()
	if err != nil {
		t.Fatal(err)
	}
	for _, deleted := range []bool{false, true} {
		if err := a.publishAccountCatalogEntry(ctx, legacy, deleted); err != nil {
			t.Fatal(err)
		}
	}
	reconcile()
	if testListed(a, "legacy") {
		t.Fatal("an empty tombstone did not remove an object without a generation")
	}
}

// TestDeleteSharedObjectKeepsRecreatedObject removes an object and creates
// one under the same id after a delete published its tombstone and before its
// local cleanup, as a catalog replay and a create in that window would. The
// delete's cleanup leaves the newer object and its acknowledged write alone.
func TestDeleteSharedObjectKeepsRecreatedObject(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	_, _, a, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()
	ref := createTestSharedObject(ctx, t, a, "race")

	// Remove the object and create it again once the delete has published.
	var hookErr error
	testHookDeletePublished = func(id string) {
		if id != "race" {
			return
		}
		relMtx, err := a.mtx.Lock(ctx)
		if err != nil {
			hookErr = err
			return
		}
		err = a.deleteSharedObjectLocked(ctx, id)
		relMtx()
		if err != nil {
			hookErr = err
			return
		}
		if _, err := a.CreateSharedObject(ctx, id, &sobject.SharedObjectMeta{BodyType: "space"}, "", ""); err != nil {
			hookErr = err
			return
		}
		fresh, releaseFresh := mountTestSharedObject(ctx, t, a, ref)
		hookErr = tryWriteTestStateData(ctx, fresh, []byte("fake-new"))
		releaseFresh()
	}
	t.Cleanup(func() { testHookDeletePublished = nil })
	err := a.DeleteSharedObject(ctx, "race")
	testHookDeletePublished = nil
	if err != nil || hookErr != nil {
		t.Fatalf("the delete returned %v, the re-create %v", err, hookErr)
	}

	// The newer object is listed, cataloged live and reads its write.
	if !testListed(a, "race") {
		t.Fatal("the delete removed the object created after its publish")
	}
	settings, err := a.readAccountSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if entry := settings.FindCatalogEntry("race"); entry.GetDeleted() || entry.GetEntry().GetGeneration() != a.listedSharedObjectEntry("race").GetGeneration() {
		t.Fatalf("the catalog does not hold the recreated object live: %v", entry)
	}
	fresh, releaseFresh := mountTestSharedObject(ctx, t, a, ref)
	defer releaseFresh()
	waitTestStateData(ctx, t, fresh, []byte("fake-new"))
}

// createTestSharedObject creates a space object with id.
func createTestSharedObject(ctx context.Context, t *testing.T, a *ProviderAccount, id string) *sobject.SharedObjectRef {
	t.Helper()
	ref, err := a.CreateSharedObject(ctx, id, &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

// mountTestSharedObject mounts ref and returns the local handle.
func mountTestSharedObject(ctx context.Context, t *testing.T, a *ProviderAccount, ref *sobject.SharedObjectRef) (*SharedObject, func()) {
	t.Helper()
	so, rel, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	return so.(*SharedObject), rel
}

// expectTestMountNotFound checks that a mount of ref is refused promptly.
func expectTestMountNotFound(ctx context.Context, t *testing.T, a *ProviderAccount, ref *sobject.SharedObjectRef) {
	t.Helper()
	mctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, rel, err := a.MountSharedObject(mctx, ref, nil)
	if err == nil {
		rel()
		t.Fatal("a mount of the deleted object succeeded")
	}
	if !errors.Is(err, sobject.ErrSharedObjectNotFound) {
		t.Fatalf("a mount of the deleted object returned %v", err)
	}
}

// writeTestStateData adopts a checkpoint holding data and waits until the
// handle's snapshot holds it.
func writeTestStateData(ctx context.Context, t *testing.T, so *SharedObject, data []byte) {
	t.Helper()
	if err := tryWriteTestStateData(ctx, so, data); err != nil {
		t.Fatal(err)
	}
	waitTestStateData(ctx, t, so, data)
}

// waitTestStateData waits until the handle's snapshot holds data. The host
// publishes the snapshot from its watch of the committed state, after the
// commit, so a read right after a write can still see the state before it.
func waitTestStateData(ctx context.Context, t *testing.T, so *SharedObject, data []byte) {
	t.Helper()
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, err := so.lsoHost.stateSnapCtr.WaitValueWithValidator(wctx, func(snap sobject.SharedObjectStateSnapshot) (bool, error) {
		if snap == nil {
			return false, nil
		}
		checkpoint, err := snap.GetCheckpoint(wctx)
		if err != nil {
			return false, err
		}
		return bytes.Equal(checkpoint.GetStateData(), data), nil
	}, nil)
	if err != nil {
		t.Fatalf("the snapshot did not show the write: %v", err)
	}
}

// tryWriteTestStateData adopts a checkpoint holding data, encrypted with the
// current key, through the handle's SOHost.
func tryWriteTestStateData(ctx context.Context, so *SharedObject, data []byte) error {
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	snapshot, err := so.GetSharedObjectState(wctx)
	if err != nil {
		return err
	}
	xfrm, err := snapshot.GetTransformer(wctx)
	if err != nil {
		return err
	}
	enc, err := xfrm.EncodeBlock(data)
	if err != nil {
		return err
	}
	id := so.GetSharedObjectID()
	return so.GetSOHost().UpdateSOState(wctx, func(next *sobject.SOState) error {
		checkpoint, err := next.BuildNextCheckpoint(id, so.GetPrivKey(), enc)
		if err != nil {
			return err
		}
		return next.AdoptCheckpoint(id, checkpoint)
	})
}

// tryWriteTestLocalKeys writes n keys to a local state store of the handle in
// one transaction.
func tryWriteTestLocalKeys(ctx context.Context, so *SharedObject, n int) error {
	local, rel, err := so.AccessLocalStateStore(ctx, "fake-store", nil)
	if err != nil {
		return err
	}
	defer rel()
	return kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
		return local.NewTransaction(ctx, true)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		for i := range n {
			if err := tx.Set(ctx, fmt.Appendf(nil, "fake-key-%04d", i), []byte("fake-value")); err != nil {
				return err
			}
		}
		return nil
	})
}

// setTestDeleteStateOff selects the old delete behavior, or the new one, for
// the rest of the test.
func setTestDeleteStateOff(t *testing.T, off bool) {
	prev := deleteStateOff
	deleteStateOff = off
	t.Cleanup(func() { deleteStateOff = prev })
}

// readTestStateData returns the state data of the handle's checkpoint.
func readTestStateData(ctx context.Context, t *testing.T, so *SharedObject) []byte {
	t.Helper()
	snapshot, err := so.GetSharedObjectState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := snapshot.GetCheckpoint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return checkpoint.GetStateData()
}

// countTestKeys counts the account object store keys under prefix.
func countTestKeys(ctx context.Context, t *testing.T, a *ProviderAccount, prefix []byte) int {
	t.Helper()
	store, rel, err := a.buildSoObjectStore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()
	var n int
	err = kvtx.RunTransaction(ctx, false, func(ctx context.Context) (kvtx.Tx, error) {
		return store.NewTransaction(ctx, false)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		n = 0
		return tx.ScanPrefixKeys(ctx, prefix, func([]byte) error {
			n++
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// testListed reports whether the account lists id.
func testListed(a *ProviderAccount, id string) bool {
	for _, entry := range a.soListCtr.GetValue().GetSharedObjects() {
		if entry.GetRef().GetProviderResourceRef().GetId() == id {
			return true
		}
	}
	return false
}
