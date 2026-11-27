package block_store_s3

import (
	"context"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_store_fillcache "github.com/s4wave/spacewave/db/block/store/fillcache"
	"github.com/sirupsen/logrus"
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

// TestDeletePackStoreReportsRemovals deletes a pack store and checks the
// decoded-block pool learns of it: a block admitted before the delete is
// reported removed before the first object goes, and one admitted while the
// delete runs is reported removed once it ends.
func TestDeletePackStoreReportsRemovals(t *testing.T) {
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

// TestDeletePackStoreInvalidatesDecodedPool reads a block of a pack store
// that keeps its packs through a fill cache with a shared decoded-block pool,
// until a hit on the cache's row admits it to the pool, then deletes the pack
// store. The cache's row survives the delete, but the store no longer holds
// the block, so no read of the row may admit it to the pool again.
func TestDeletePackStoreInvalidatesDecodedPool(t *testing.T) {
	// Put a block in a pack store that keeps its packs, under a prefix no
	// other test deletes.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	_, client := newFakeBucket(t, nil)
	const prefix = "pool/"
	store := NewPackStore(le, client, "bucket", prefix, &PackStoreOptions{KeepPacks: true})
	t.Cleanup(store.Close)
	ref, _, err := block.PutBlock(ctx, store, &block_mock.Example{Msg: "kept"})
	if err != nil {
		t.Fatal(err)
	}

	// Read the block through a fill cache over the store with a pool.
	cache, err := block_store_fillcache.Open(ctx, le, filepath.Join(t.TempDir(), "fill.db"), block_store_fillcache.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	pool, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	reads := cache.Reads(func() block.StoreOps { return store })
	read := func() {
		t.Helper()
		tx, cursor := block.NewTransaction(reads(), nil, ref, nil)
		tx.SetDecodedBlockCache(pool)
		if _, err := cursor.Unmarshal(ctx, block_mock.NewExampleBlock); err != nil {
			t.Fatal(err)
		}
	}

	// The first read fills the cache. A hit on the row then admits the block,
	// and the next read takes it from the pool.
	read()
	if err := cache.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	stores := pool.Snapshot().Stores
	read()
	if got := pool.Snapshot().Stores; got != stores+1 {
		t.Fatalf("a hit on the cache over a store keeping its packs stored %d blocks into the pool, want 1", got-stores)
	}
	hits := pool.Snapshot().Hits
	read()
	if got := pool.Snapshot().Hits; got != hits+1 {
		t.Fatalf("the read after admission missed the pool: hits %d -> %d", hits, got)
	}

	// Delete the pack store. The reads after it are served by the cache's
	// surviving row, as the store holds nothing, and neither may admit the
	// block or take it from the pool.
	if err := DeletePackStore(ctx, client, "bucket", prefix); err != nil {
		t.Fatal(err)
	}
	before := pool.Snapshot()
	read()
	read()
	after := pool.Snapshot()
	if after.Stores != before.Stores {
		t.Fatalf("reads of the surviving row stored %d blocks into the pool after the delete", after.Stores-before.Stores)
	}
	if after.Hits != before.Hits {
		t.Fatalf("the pool served a block of the deleted store: hits %d -> %d", before.Hits, after.Hits)
	}
	if store.KeepsBlocks() {
		t.Fatal("a deleted pack store reports keeping its blocks")
	}
}
