package block_store_s3

import (
	"context"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

// TestDeletePackStore deletes every object of one store and none of another
// store's in the same bucket.
func TestDeletePackStore(t *testing.T) {
	// Write two packs alongside another store object in the fake bucket.
	ctx := t.Context()
	bucket, client := newFakeBucket(t, map[string]string{"q/packs/other": "x"})
	store := newTestPackStore(t, client)
	for _, data := range []string{"one", "two"} {
		if _, _, err := store.PutBlock(ctx, []byte(data), nil); err != nil {
			t.Fatal(err)
		}
	}

	// Delete the pack store and verify the other store object survives.
	if err := DeletePackStore(ctx, client, "bucket", "p/"); err != nil {
		t.Fatal(err)
	}
	if keys := bucket.keys(); !slices.Equal(keys, []string{"q/packs/other"}) {
		t.Fatalf("keys = %v; want only the other store's object", keys)
	}

	// A fresh store finds no blocks, and a rerun deletes nothing more.
	ref, err := block.BuildBlockRef([]byte("one"), nil)
	if err != nil {
		t.Fatal(err)
	}
	exists, err := newTestPackStore(t, client).GetBlockExists(ctx, ref)
	if err != nil || exists {
		t.Fatalf("exists = %v, %v; want the block gone", exists, err)
	}
	if err := DeletePackStore(ctx, client, "bucket", "p/"); err != nil {
		t.Fatal(err)
	}
}

// deleteProbe calls onDelete before each object delete.
type deleteProbe struct {
	ObjectStore
	onDelete func()
}

// DeleteObject calls onDelete, then deletes the object.
func (d *deleteProbe) DeleteObject(ctx context.Context, bucket, key string) error {
	d.onDelete()
	return d.ObjectStore.DeleteObject(ctx, bucket, key)
}

// TestDeletePackStoreInvalidatesDecodedPool deletes a pack store and checks
// the decoded-block pool learns of it: a block admitted before the delete is
// reported removed before the first object goes, and one admitted while the
// delete runs is reported removed once it ends.
func TestDeletePackStoreInvalidatesDecodedPool(t *testing.T) {
	// Write a pack and note the generation of a read before the delete.
	ctx := t.Context()
	_, client := newFakeBucket(t, map[string]string{})
	store := newTestPackStore(t, client)
	ref, _, err := store.PutBlock(ctx, []byte("one"), nil)
	if err != nil {
		t.Fatal(err)
	}
	before := block.DurableReadGeneration()

	// Check at each object delete that the earlier read is reported, and note
	// the generation of a read during the delete.
	var deletes atomic.Int64
	var during atomic.Uint64
	probe := &deleteProbe{ObjectStore: client, onDelete: func() {
		deletes.Add(1)
		if !block.RemovedSince(ref, before) {
			t.Error("an object was deleted before the block was reported removed")
		}
		during.Store(block.DurableReadGeneration())
	}}
	if err := DeletePackStore(ctx, probe, "bucket", "p/"); err != nil {
		t.Fatal(err)
	}
	if deletes.Load() == 0 {
		t.Fatal("the delete removed no object")
	}
	if !block.RemovedSince(ref, during.Load()) {
		t.Fatal("a read during the delete was not reported removed after it")
	}
}
