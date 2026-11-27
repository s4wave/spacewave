package provider_local

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
)

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
