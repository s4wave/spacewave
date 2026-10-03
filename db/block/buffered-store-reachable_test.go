package block

import (
	"context"
	"errors"
	"testing"
)

// TestBufferedStoreSyncReachable retains shared children and fences the final
// DAG while discarding superseded content without deleting durable blocks.
func TestBufferedStoreSyncReachable(t *testing.T) {
	// Create a buffered store and a helper for referenced block writes.
	ctx := t.Context()
	inner := newSyncOrderStore(0)
	store := NewBufferedStore(ctx, inner)
	put := func(value string, refs ...*BlockRef) *BlockRef {
		t.Helper()
		ref, _, err := store.PutBlock(ctx, []byte(value), &PutOpts{Refs: refs})
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}

	// Queue a shared-child graph with a superseded parent.
	child := put("shared child")
	old := put("superseded", child)
	left := put("left", child)
	right := put("right", child)
	root := put("root", left, right)
	if len(inner.blocks) != 0 {
		t.Fatal("construction wrote to storage")
	}

	// Verify cancellation aborts the reachable-graph fence.
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.SyncReachable(canceled, root); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled commit: %v", err)
	}

	// Flush the final reachable graph and verify all retained blocks persist.
	if _, err := store.SyncReachable(ctx, root); err != nil {
		t.Fatal(err)
	}
	for _, ref := range []*BlockRef{child, left, right, root} {
		found, err := inner.GetBlockExists(ctx, ref)
		if err != nil || !found {
			t.Fatalf("missing reachable block: %v", err)
		}
	}

	// Verify the superseded block is skipped and the durability fence runs.
	if found, err := inner.GetBlockExists(ctx, old); err != nil || found {
		t.Fatalf("superseded block was copied: found=%v err=%v", found, err)
	}
	events := inner.snapshotEvents()
	if len(events) == 0 || events[len(events)-1] != "sync" {
		t.Fatalf("durability fence missing: %v", events)
	}
}
