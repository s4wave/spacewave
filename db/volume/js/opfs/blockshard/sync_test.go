//go:build js

package blockshard

import (
	"context"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/volume/js/opfs/segment"
)

// TestSyncBarrierFencesPriorWrites proves a Sync barrier returns only after the
// publish covering earlier-enqueued writes is durable: the store reports a fence
// applied, the published generation does not regress, and the value is readable.
func TestSyncBarrierFencesPriorWrites(t *testing.T) {
	settings := DefaultSettings()
	settings.ShardCount = 1
	settings.AsyncIO = true
	e, cleanup := newTestEngineWithSettings(t, "test-blockshard-sync-fence", "test-blockshard-sync-fence", settings)
	defer cleanup()

	store := NewBlockStore(e, block.DefaultHashType)
	ref, _, err := store.PutBlock(context.Background(), []byte("durable"), nil)
	if err != nil {
		t.Fatal(err)
	}
	genAfterPut := e.shards[0].Manifest().Generation

	fenced, err := store.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !fenced {
		t.Fatal("Sync over the shard engine must report a fence applied")
	}
	if gen := e.shards[0].Manifest().Generation; gen < genAfterPut {
		t.Fatalf("generation after Sync = %d, want >= %d", gen, genAfterPut)
	}

	val, found, err := store.GetBlock(context.Background(), ref.Clone())
	if err != nil {
		t.Fatal(err)
	}
	if !found || string(val) != "durable" {
		t.Fatalf("post-Sync read: found=%v val=%q want durable", found, val)
	}
}

// TestSyncOnIdleShardEmitsNoEmptyPublish proves a barrier over an idle shard
// satisfies the fence without bumping the generation: the actor must not emit an
// empty publish when no new entries arrived since the last publish.
func TestSyncOnIdleShardEmitsNoEmptyPublish(t *testing.T) {
	settings := DefaultSettings()
	settings.ShardCount = 1
	settings.AsyncIO = true
	e, cleanup := newTestEngineWithSettings(t, "test-blockshard-sync-idle", "test-blockshard-sync-idle", settings)
	defer cleanup()

	if err := e.Put(context.Background(), []segment.Entry{{
		Key:   []byte("seed"),
		Value: []byte("value"),
	}}); err != nil {
		t.Fatal(err)
	}
	genBefore := e.shards[0].Manifest().Generation

	for range 3 {
		if err := e.Sync(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if gen := e.shards[0].Manifest().Generation; gen != genBefore {
		t.Fatalf("idle Sync bumped generation: got %d want %d (no empty publish)", gen, genBefore)
	}
}

// TestSyncFencesAllShards proves Sync dispatches a barrier to every shard and
// returns only after all replies, then a second idle Sync bumps no shard
// generation and every prior write remains durable.
func TestSyncFencesAllShards(t *testing.T) {
	settings := DefaultSettings()
	settings.ShardCount = 4
	settings.AsyncIO = true
	e, cleanup := newTestEngineWithSettings(t, "test-blockshard-sync-all-shards", "test-blockshard-sync-all-shards", settings)
	defer cleanup()

	store := NewBlockStore(e, block.DefaultHashType)
	refs := make([]*block.BlockRef, 0, 16)
	for i := range 16 {
		ref, _, err := store.PutBlock(context.Background(), []byte("v-"+strconv.Itoa(i)), nil)
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, ref)
	}

	fenced, err := store.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !fenced {
		t.Fatal("multi-shard Sync must report a fence applied")
	}

	gens := make([]uint64, len(e.shards))
	for i := range e.shards {
		gens[i] = e.shards[i].Manifest().Generation
	}
	if _, err := store.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	for i := range e.shards {
		if gen := e.shards[i].Manifest().Generation; gen != gens[i] {
			t.Fatalf("idle Sync bumped shard %d generation: got %d want %d", i, gen, gens[i])
		}
	}

	for i, ref := range refs {
		val, found, err := store.GetBlock(context.Background(), ref.Clone())
		if err != nil {
			t.Fatal(err)
		}
		if !found || string(val) != "v-"+strconv.Itoa(i) {
			t.Fatalf("post-Sync read %d: found=%v val=%q", i, found, val)
		}
	}
}
