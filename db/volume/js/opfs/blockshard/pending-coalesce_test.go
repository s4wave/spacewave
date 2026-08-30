//go:build js

package blockshard

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
)

// TestPutPendingBatchesCoalesceUntilSync proves consecutive PutPending batches
// stay readable through the pending buffer before publication, coalesce into
// fewer publish cycles, and become durable after one Sync.
func TestPutPendingBatchesCoalesceUntilSync(t *testing.T) {
	settings := DefaultSettings()
	settings.ShardCount = 1
	e, cleanup := newTestEngineWithSettings(t, "test-blockshard-pending-coalesce", "test-blockshard-pending-coalesce", settings)
	defer cleanup()

	store := NewBlockStore(e, block.DefaultHashType)
	ctx := context.Background()

	const batches = 8
	refs := make([]*block.BlockRef, 0, batches)
	for i := range batches {
		entries := []*block.PutBatchEntry{{
			Ref:  makeCoalesceRef(t, i),
			Data: []byte(fmt.Sprintf("pending-%d", i)),
		}}
		if err := store.PutBlockBatch(ctx, entries); err != nil {
			t.Fatal(err)
		}
		refs = append(refs, entries[0].Ref)
	}

	// Before Sync: reads succeed through the pending buffer regardless of
	// whether the actor has published yet.
	time.Sleep(20 * time.Millisecond)
	for i, ref := range refs {
		val, found, err := store.GetBlock(ctx, ref.Clone())
		if err != nil {
			t.Fatal(err)
		}
		if !found || string(val) != fmt.Sprintf("pending-%d", i) {
			t.Fatalf("pending read %d: found=%v val=%q", i, found, val)
		}
	}

	// One Sync fences every batch durable.
	if err := e.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if gen := e.shards[0].Manifest().Generation; gen == 0 {
		t.Fatal("Sync did not publish pending batches")
	}
	for i, ref := range refs {
		val, found, err := store.GetBlock(ctx, ref.Clone())
		if err != nil {
			t.Fatal(err)
		}
		if !found || string(val) != fmt.Sprintf("pending-%d", i) {
			t.Fatalf("post-Sync read %d: found=%v val=%q", i, found, val)
		}
	}
}

func makeCoalesceRef(t *testing.T, i int) *block.BlockRef {
	t.Helper()
	data := []byte(fmt.Sprintf("pending-%d", i))
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}
