package volume_controller

import (
	"bytes"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/bucket"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
)

// TestBucketRemovalPreservesSharedBlock exercises logical release through both
// entry points, then lets the volume collector reclaim the final orphan.
func TestBucketRemovalPreservesSharedBlock(t *testing.T) {
	for _, batch := range []bool{false, true} {
		name := "direct"
		if batch {
			name = "batch"
		}
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			vol, err := volume_kvtx.NewVolume(ctx, "test", store_kvkey.NewDefaultKVKey(), store_inmem.NewStore(), nil, false, false, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := vol.Close(); err != nil {
					t.Error(err)
				}
			})
			rg := vol.GetRefGraph()
			makeBucket := func(id string) *bucketHandle {
				t.Helper()
				if err := rg.AddRef(ctx, block_gc.NodeGCRoot, block_gc.BucketIRI(id)); err != nil {
					t.Fatal(err)
				}
				return &bucketHandle{
					t:          &bucketHandleTracker{bucketID: id},
					v:          vol,
					bucketConf: &bucket.Config{Id: id},
					gcOps:      block_gc.NewGCStoreOpsWithParent(vol, rg, block_gc.BucketIRI(id)),
				}
			}
			a, b := makeBucket("a"), makeBucket("b")

			// Both owners retain one physical content-addressed block.
			data := []byte("shared payload")
			ref, _, err := a.PutBlock(ctx, data, nil)
			if err != nil {
				t.Fatal(err)
			}
			other, existed, err := b.PutBlock(ctx, data, nil)
			if err != nil || !existed || !ref.EqualVT(other) {
				t.Fatalf("deduplicated put: existed=%t err=%v", existed, err)
			}
			remove := func(handle *bucketHandle) {
				t.Helper()
				var err error
				if batch {
					err = handle.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Tombstone: true}})
				} else {
					err = handle.RmBlock(ctx, ref)
				}
				if err != nil {
					t.Fatal(err)
				}
			}

			// One release removes ownership before returning without losing B.
			remove(a)
			owners, err := rg.GetIncomingRefs(ctx, block_gc.BlockIRI(ref))
			if err != nil || !slices.Equal(owners, []string{block_gc.BucketIRI("b")}) {
				t.Fatalf("owners after release: %v err=%v", owners, err)
			}
			collector := block_gc.NewCollector(rg, vol, nil)
			if _, err := collector.Collect(ctx); err != nil {
				t.Fatal(err)
			}
			got, found, err := b.GetBlock(ctx, ref)
			if err != nil || !found || !bytes.Equal(data, got) {
				t.Fatalf("shared block after collection: found=%t err=%v", found, err)
			}

			// Releasing the last owner permits physical collection.
			remove(b)
			if _, err := collector.Collect(ctx); err != nil {
				t.Fatal(err)
			}
			if _, found, err := vol.GetBlock(ctx, ref); err != nil || found {
				t.Fatalf("orphan after collection: found=%t err=%v", found, err)
			}
		})
	}
}
