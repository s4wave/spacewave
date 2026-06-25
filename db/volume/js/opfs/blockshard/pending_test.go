//go:build js

package blockshard

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/volume/js/opfs/segment"
)

// TestDefaultWriteReadsBackBeforePublish proves the continuous-write path: a
// default write returns before it is published, reads back from the engine
// buffer while still unpublished (generation unchanged), and Sync fences it so
// the buffer fully drains and the value persists in the manifest.
func TestDefaultWriteReadsBackBeforePublish(t *testing.T) {
	settings := DefaultSettings()
	settings.ShardCount = 1
	e, cleanup := newTestEngineWithSettings(t, "test-blockshard-pending-raw", "test-blockshard-pending-raw", settings)
	defer cleanup()

	store := NewBlockStore(e, block.DefaultHashType)
	ref, existed, err := store.PutBlock(context.Background(), []byte("buffered"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if existed {
		t.Fatal("background put should report a new block")
	}

	// Single-threaded wasm: the write actor runs only when this goroutine
	// yields, so right after the non-blocking put the write is still buffered and
	// the manifest generation has not advanced (no synchronous per-write publish).
	if n := e.pending[0].length(); n != 1 {
		t.Fatalf("pending buffer length after background put: got %d want 1", n)
	}
	if gen := e.shards[0].Manifest().Generation; gen != 0 {
		t.Fatalf("generation after non-blocking background put: got %d want 0 (no publish)", gen)
	}

	val, found, err := store.GetBlock(context.Background(), ref.Clone())
	if err != nil {
		t.Fatal(err)
	}
	if !found || string(val) != "buffered" {
		t.Fatalf("read-after-write before publish: found=%v val=%q want buffered", found, val)
	}

	fenced, err := store.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !fenced {
		t.Fatal("Sync must report a fence applied")
	}
	if n := e.pending[0].length(); n != 0 {
		t.Fatalf("pending buffer length after Sync: got %d want 0 (fully evicted)", n)
	}
	if gen := e.shards[0].Manifest().Generation; gen == 0 {
		t.Fatal("generation after Sync: got 0 want a publish")
	}

	val, found, err = store.GetBlock(context.Background(), ref.Clone())
	if err != nil {
		t.Fatal(err)
	}
	if !found || string(val) != "buffered" {
		t.Fatalf("read after Sync eviction: found=%v val=%q want buffered", found, val)
	}
}

// TestPendingTombstoneShadowsPublishedValue proves pending-then-published read
// order: a buffered tombstone wins over an older published value across the
// value, existence, and batch-existence read paths.
func TestPendingTombstoneShadowsPublishedValue(t *testing.T) {
	settings := DefaultSettings()
	settings.ShardCount = 1
	e, cleanup := newTestEngineWithSettings(t, "test-blockshard-pending-tombstone", "test-blockshard-pending-tombstone", settings)
	defer cleanup()

	store := NewBlockStore(e, block.DefaultHashType)
	ref, _, err := store.PutBlock(context.Background(), []byte("live"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.GetBlock(context.Background(), ref.Clone()); err != nil {
		t.Fatal(err)
	} else if !found {
		t.Fatal("precondition: published block should exist")
	}

	key, err := encodeRef(ref)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.PutBackground(context.Background(), []segment.Entry{{
		Key:       []byte(key),
		Tombstone: true,
	}}); err != nil {
		t.Fatal(err)
	}
	if n := e.pending[0].length(); n != 1 {
		t.Fatalf("pending buffer length after background tombstone: got %d want 1", n)
	}

	if _, found, err := store.GetBlock(context.Background(), ref.Clone()); err != nil {
		t.Fatal(err)
	} else if found {
		t.Fatal("buffered tombstone must shadow the published value (GetBlock)")
	}
	if found, err := store.GetBlockExists(context.Background(), ref.Clone()); err != nil {
		t.Fatal(err)
	} else if found {
		t.Fatal("buffered tombstone must shadow existence (GetBlockExists)")
	}
	found, err := store.GetBlockExistsBatch(context.Background(), []*block.BlockRef{ref.Clone()})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0] {
		t.Fatalf("buffered tombstone must shadow batch existence: got %v want [false]", found)
	}
}
