package provider_local

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
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
	// serves the deleted state once the delete returns.
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
	testHookDeleteCommitted.Store(new(func(id string) {
		if id != "racing" {
			return
		}
		ranHook = true
		if so.ctx.Err() != nil {
			t.Error("the mount ended before the delete restarted it")
		}
		stateErr = tryWriteTestStateData(ctx, so, []byte("fake-racing"))
		localErr = tryWriteTestLocalKeys(ctx, so, 1)
	}))
	t.Cleanup(func() { testHookDeleteCommitted.Store(nil) })
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
	testHookDeletePublished.Store(new(func(id string) {
		if id == "rerun" {
			published = true
			dcancel()
		}
	}))
	t.Cleanup(func() { testHookDeletePublished.Store(nil) })
	err = a.DeleteSharedObject(dctx, "rerun")
	testHookDeletePublished.Store(nil)

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
// only its genesis. State an enrollment is writing stays and fails the
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
		// An enrollment that returned without listing the id left the
		// placeholder and its hold, which the create releases.
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
		// the write retried once. Keys that keep appearing fail the
		// write.
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
	err = a.ReplaceSharedObjectState(ctx, a.le, "unlisted", owner, []byte("fake-world"), ReplaceOrigin{})
	if !errors.Is(err, sobject.ErrSharedObjectNotFound) {
		t.Fatalf("a transfer write to an unlisted id returned %v", err)
	}
	if n := countTestKeys(ctx, t, a, SobjectObjectStorePrefix("unlisted")); n != 0 {
		t.Fatalf("a transfer write to an unlisted id left %d keys", n)
	}

	// An enrollment with an id that is not a DNS label fails before writing.
	// Its ref check is the only one before the placeholder write: the genesis
	// builder does not validate the id, so without the ref check keys land
	// under so/a/.
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
	testHookDeletePublished.Store(new(func(id string) {
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
	}))
	t.Cleanup(func() { testHookDeletePublished.Store(nil) })
	err := a.DeleteSharedObject(ctx, "race")
	testHookDeletePublished.Store(nil)
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
