//go:build darwin || linux || windows

package kvtx

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	db_kvtx "github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// TestVolumeSharedCountsConfig opens one store of format 5 that tracks blocks
// under each Config in turn: without the upgrade flag it stays at format 5
// and keeps no counts; the flag moves it to format 6 and begins the walk;
// disabling the counts stops the walk; enabling them again begins a new one,
// which a block pass runs to the end. The conflicting pair is refused, and an
// empty store moves to format 6 at once without the flag.
func TestVolumeSharedCountsConfig(t *testing.T) {
	// Write a format 5 store whose graph tracks a block.
	ctx := t.Context()
	store := store_kvtx_inmem.NewStore()
	keys := store_kvkey.NewDefaultKVKey()
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, keys.GetFormatVersionKey(), store_kvkey.MarshalFormatVersion(5)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := OpenRefGraph(store).AddRef(ctx, block_gc.BucketIRI("bucket"), testBlockIRI(t, "a")); err != nil {
		t.Fatal(err)
	}

	// open opens and closes a Volume over the store, checks its format, its
	// counts and its walk's phase, and runs fn while it is open.
	open := func(conf *store_kvtx.Config, format uint64, counts bool, phase block_gc.SharingPhase, fn func(*Volume)) {
		// Open the Volume.
		t.Helper()
		v, err := NewVolume(ctx, "test/sharing", keys, store, conf, false, false, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := v.Close(); err != nil {
				t.Error(err)
			}
		}()

		// Check its format, counts and walk.
		got, err := readFormatVersion(ctx, store, keys)
		if err != nil || got != format {
			t.Fatalf("format %d err %v, want %d", got, err, format)
		}
		if v.holdCounts != counts {
			t.Fatalf("hold counts %v, want %v", v.holdCounts, counts)
		}
		checkSharingPhase(t, store, phase)
		if fn != nil {
			fn(v)
		}
	}

	// Without the flag the store stays at format 5 and keeps no counts.
	open(&store_kvtx.Config{}, 5, false, block_gc.SharingComplete, nil)

	// The conflicting pair is refused.
	conflict := &store_kvtx.Config{DisableHoldCounts: true, UpgradeSharedCounts: true}
	if _, err := NewVolume(ctx, "test/sharing", keys, store, conflict, false, false, nil, nil); err == nil {
		t.Fatal("opened with disable_hold_counts and upgrade_shared_counts")
	}

	// The flag moves the store to format 6 and begins the walk.
	open(&store_kvtx.Config{UpgradeSharedCounts: true}, store_kvkey.FormatVersion, true, block_gc.SharingClearingHolds, nil)

	// Disabling the counts stops the walk.
	open(&store_kvtx.Config{DisableHoldCounts: true}, store_kvkey.FormatVersion, false, block_gc.SharingOff, nil)

	// Enabling them again begins a new walk, which a block pass completes.
	open(&store_kvtx.Config{}, store_kvkey.FormatVersion, true, block_gc.SharingClearingHolds, func(v *Volume) {
		if err := v.walkSharing(ctx); err != nil {
			t.Fatal(err)
		}
		checkSharingPhase(t, store, block_gc.SharingComplete)
	})

	// An empty store moves to format 6 at once and keeps counts.
	store = store_kvtx_inmem.NewStore()
	open(&store_kvtx.Config{}, store_kvkey.FormatVersion, true, block_gc.SharingComplete, nil)
}

// TestSharingWalkDeadlistRenames names a deadlist record whose old root a
// head also names, and renames it due at each point of a walk run in
// one-record slices, then finishes the walk: wherever the cursor sits, the
// old root's count covers both holders. Implied answers the old root for a
// pending or a due record, nothing for another name or a pin, and an error
// for a record it cannot read.
func TestSharingWalkDeadlistRenames(t *testing.T) {
	// Encode the old root and a record naming it.
	ctx := t.Context()
	from, err := block.BuildBlockRef([]byte("old root"), nil)
	if err != nil {
		t.Fatal(err)
	}
	gone, err := block.BuildBlockRef([]byte("dead page"), nil)
	if err != nil {
		t.Fatal(err)
	}
	record, recordRef, err := block.EncodeDeadlist(1, from, nil, &block.Replacements{Removed: []*block.BlockRef{gone}})
	if err != nil {
		t.Fatal(err)
	}

	// Store the old root and the record under the pending name, which a
	// rename moves due.
	entries := append([]*block.PutBatchEntry{{Ref: from, Data: []byte("old root"), RefsKnown: true}}, record...)
	pending := []block.NamedRoot{{Name: "head", Ref: from}, {Name: block.DeadlistRootName(1), Ref: recordRef}}
	due := []block.NamedRoot{{Name: block.DeadlistRootName(1)}, {Name: block.DeadlistDueRootName(1), Ref: recordRef}}

	// Rename after each count of slices, until the walk ends before the
	// rename.
	for slices := 0; ; slices++ {
		// Name the record and begin a walk.
		v, _ := newPublicationTestVolume(t)
		if err := v.SetBucketRoots(ctx, "bucket", entries, pending); err != nil {
			t.Fatal(err)
		}
		rg := OpenRefGraph(v.kvtxStore)
		if err := rg.StopSharing(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := rg.BeginSharing(ctx); err != nil {
			t.Fatal(err)
		}

		// Ask Implied about each owner of the record once.
		if slices == 0 {
			checkImplied(t, v, recordRef, gone, block_gc.BlockIRI(from))
		}

		// Walk the slices, rename the record due, and finish the walk.
		running := true
		for range slices {
			err := v.withDirectAtomic(ctx, func(_ block.StoreOps, rg *block_gc.RefGraph) error {
				s, err := rg.WalkSharing(ctx, func() bool { return true })
				running = s.Phase.Running()
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			if !running {
				break
			}
		}
		if err := v.SetBucketRoots(ctx, "bucket", nil, due); err != nil {
			t.Fatal(err)
		}
		if err := v.walkSharing(ctx); err != nil {
			t.Fatal(err)
		}

		// The head and the due record hold the old root.
		err := (&transactionRefGraph{volume: v}).read(ctx, func(rg *block_gc.RefGraph) error {
			n, err := rg.GetHolds(ctx, block_gc.BlockIRI(from))
			if err == nil && n < 1 {
				t.Errorf("renamed after %d slices: old root holds %d, want at least 1", slices, n)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if !running {
			return
		}
	}
}

// checkImplied checks what Implied answers for each owner of the record at
// ref, whose old root is from, and for a record it cannot read at missing.
func checkImplied(t *testing.T, v *Volume, ref, missing *block.BlockRef, from string) {
	t.Helper()
	ctx := t.Context()
	err := v.withDirectAtomic(ctx, func(ops block.StoreOps, _ *block_gc.RefGraph) error {
		// Each owner of the record.
		h := txHolders{blocks: ops}
		for owner, want := range map[string]string{
			RootOwnerIRI("bucket", block.DeadlistRootName(1)):    from,
			RootOwnerIRI("bucket", block.DeadlistDueRootName(1)): from,
			RootOwnerIRI("bucket", "head"):                       "",
			rootPinPrefix + "reader":                             "",
		} {
			got, err := h.Implied(ctx, owner, block_gc.BlockIRI(ref))
			if err != nil || got != want {
				t.Errorf("Implied(%s) = %q %v, want %q", owner, got, err, want)
			}
		}

		// A record it cannot read.
		if _, err := h.Implied(ctx, RootOwnerIRI("bucket", block.DeadlistDueRootName(2)), block_gc.BlockIRI(missing)); err == nil {
			t.Error("Implied read a missing record")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// checkSharingPhase checks the phase of the sharing walk in store's graph.
func checkSharingPhase(t *testing.T, store db_kvtx.Store, phase block_gc.SharingPhase) {
	t.Helper()
	s, err := OpenRefGraph(store).GetSharing(context.Background())
	if err != nil || s.Phase != phase {
		t.Fatalf("sharing phase %c err %v, want %c", s.Phase, err, phase)
	}
}
