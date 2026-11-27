package provider_local

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
)

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
