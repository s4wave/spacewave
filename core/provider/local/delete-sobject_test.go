package provider_local

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
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
