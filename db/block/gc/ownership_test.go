package block_gc_test

import (
	"bytes"
	"context"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	common_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
)

func TestGCStoreOpsDeduplicatedBlockRetainsBucketOwner(t *testing.T) {
	for _, batch := range []bool{false, true} {
		// Choose the shared-block write path for this test case.
		name := "put"
		if batch {
			name = "batch"
		}
		t.Run(name, func(t *testing.T) {
			// Create the volume key for the shared-bucket ownership test.
			ctx := context.Background()
			kvKey, err := store_kvkey.NewKVKey(store_kvkey.DefaultConfig())
			if err != nil {
				t.Fatalf("new kv key: %v", err)
			}

			// Open the volume and register its cleanup.
			vol, err := common_kvtx.NewVolume(
				ctx,
				"gc-deduplicated-owner-test",
				kvKey,
				store_kvtx_inmem.NewStore(),
				&store_kvtx.Config{},
				false,
				false,
				nil,
				nil,
			)
			if err != nil {
				t.Fatalf("new volume: %v", err)
			}
			t.Cleanup(func() {
				if err := vol.Close(); err != nil {
					t.Fatalf("close volume: %v", err)
				}
			})

			// Register two rooted buckets in the volume reference graph.
			rg := vol.GetRefGraph()
			providerIRI := "provider:test"
			bucketAIRI := block_gc.BucketIRI("bucket-a")
			bucketBIRI := block_gc.BucketIRI("bucket-b")
			for _, bucketIRI := range []string{bucketAIRI, bucketBIRI} {
				// Root the bucket in the reference graph.
				if err := rg.AddRef(ctx, block_gc.NodeGCRoot, bucketIRI); err != nil {
					t.Fatalf("root bucket: %v", err)
				}

				// Connect the provider to the bucket.
				if err := rg.AddRef(ctx, providerIRI, bucketIRI); err != nil {
					t.Fatalf("own bucket: %v", err)
				}
			}

			// Create bucket-owned stores and encode their shared block.
			bucketA := block_gc.NewGCStoreOpsWithParent(vol, rg, bucketAIRI)
			bucketB := block_gc.NewGCStoreOpsWithParent(vol, rg, bucketBIRI)
			example := block_mock.NewExample("shared bucket block")
			data, err := example.MarshalBlock()
			if err != nil {
				t.Fatalf("marshal block: %v", err)
			}

			// Define the selected write path for a bucket-owned block.
			put := func(store *block_gc.GCStoreOps) *block.BlockRef {
				// Write the shared block through the selected batch path.
				t.Helper()
				if batch {
					ref, err := block.BuildBlockRef(data, nil)
					if err != nil {
						t.Fatalf("build block ref: %v", err)
					}

					// Write the encoded block as a batch entry.
					entry := &block.PutBatchEntry{Ref: ref, Data: data}
					if err := store.PutBlockBatch(ctx, []*block.PutBatchEntry{entry}); err != nil {
						t.Fatalf("put block batch: %v", err)
					}
					return ref
				}

				// Write the encoded block through the single-block path.
				ref, _, err := store.PutBlock(ctx, data, nil)
				if err != nil {
					t.Fatalf("put block: %v", err)
				}
				return ref
			}

			// Define the bucket ownership edge assertion.
			assertOwns := func(name, parent string, ref *block.BlockRef) {
				t.Helper()
				outgoing, err := rg.GetOutgoingRefs(ctx, parent)
				if err != nil {
					t.Fatalf("get %s ownership edges: %v", name, err)
				}
				if !slices.Contains(outgoing, block_gc.BlockIRI(ref)) {
					t.Fatalf("%s did not record its block ownership edge", name)
				}
			}

			// Write and flush the shared block under bucket A.
			refA := put(bucketA)
			if err := bucketA.FlushPending(ctx); err != nil {
				t.Fatalf("flush bucket A: %v", err)
			}

			// Write and flush the shared block under bucket B.
			refB := put(bucketB)
			if err := bucketB.FlushPending(ctx); err != nil {
				t.Fatalf("flush bucket B: %v", err)
			}

			// Verify both buckets store the same block reference.
			if !refA.EqualVT(refB) {
				t.Fatalf("expected identical block refs, got %v and %v", refA, refB)
			}

			// Release bucket A from both graph roots and collect its orphaned nodes.
			gcOps := block_gc.NewGCStoreOps(vol, rg)
			if err := gcOps.RemoveGCRef(ctx, block_gc.NodeGCRoot, bucketAIRI); err != nil {
				t.Fatalf("remove bucket root: %v", err)
			}
			if err := gcOps.RemoveGCRef(ctx, providerIRI, bucketAIRI); err != nil {
				t.Fatalf("remove provider owner: %v", err)
			}
			if _, err := block_gc.NewCollector(rg, vol, nil).Collect(ctx); err != nil {
				t.Fatalf("collect account blocks: %v", err)
			}

			// Read the shared block from the surviving bucket.
			got, exists, err := bucketB.GetBlock(ctx, refB)
			if err != nil {
				t.Fatalf("read surviving bucket block: %v", err)
			}

			// Verify the surviving bucket block retains its payload.
			if !exists {
				t.Fatal("surviving bucket block was physically deleted")
			}
			if !bytes.Equal(got, data) {
				t.Fatal("surviving bucket block data changed")
			}

			// Verify bucket B retains its block ownership edge.
			assertOwns("bucket B", bucketBIRI, refB)
		})
	}
}
