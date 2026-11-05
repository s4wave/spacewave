package s4wave_kv_world_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/testbed"
)

// TestWorldBackedStoreReleasesCommittedRoots checks that a long-lived store
// drops its stage's ownership of each root once the World references it, so
// the stage does not retain every superseded root until Close.
func TestWorldBackedStoreReleasesCommittedRoots(t *testing.T) {
	// Start a testbed.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Open a KV store on an empty object.
	objectKey := "kv/stage-release"
	createEmptyKvStoreObject(t, ctx, tb.WorldState, objectKey)
	store, closeFn := openWorldBackedStore(t, ctx, tb.WorldState, objectKey)
	defer closeFn()

	// Commit several transactions and record each committed root.
	var roots []*block.BlockRef
	for i := range 3 {
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, []byte("key"), []byte(strconv.Itoa(i))); err != nil {
			tx.Discard()
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			tx.Discard()
			t.Fatal(err)
		}
		tx.Discard()
		roots = append(roots, getObjectRoot(t, ctx, tb.WorldState, objectKey).GetRootRef())
	}

	// No stage holds a committed root while the store stays open.
	err = tb.Volume.ListRoots(ctx, "stage:", func(root block.NamedRoot) error {
		for _, committed := range roots {
			if root.Ref.EqualsRef(committed) {
				t.Fatalf("stage root %s still holds committed root %s", root.Name, committed.MarshalString())
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
