package bucket_lookup

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/byteslice"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_mock "github.com/s4wave/spacewave/db/bucket/mock"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/runtimeenv"
)

// copyDestination observes the copy's storage boundary and injects failures.
type copyDestination struct {
	// inner supplies real in-memory block storage.
	inner block.StoreOps
	// batchSizes records the submitted ownership batches.
	batchSizes []int
	// syncs counts completed-copy durability requests.
	syncs int
	// batchErr rejects a destination write batch when set.
	batchErr error
	// syncErr rejects the final durability fence when set.
	syncErr error
	// singleProbes counts individual destination existence operations.
	singleProbes atomic.Int64
	// batchProbes counts batched destination existence operations.
	batchProbes atomic.Int64
}

// GetBlockExists counts individual destination probes.
func (d *copyDestination) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	d.singleProbes.Add(1)
	return d.inner.GetBlockExists(ctx, ref)
}

// GetBlockExistsBatch counts batched destination probes.
func (d *copyDestination) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	d.batchProbes.Add(1)
	return d.inner.GetBlockExistsBatch(ctx, refs)
}

// TestCopyObjectLargerThanBuffer preserves valid single-block copies that exceed
// the batching budget, rather than waiting forever for impossible capacity.
func TestCopyObjectLargerThanBuffer(t *testing.T) {
	for _, engine := range []runtimeenv.Engine{runtimeenv.EngineChromium, runtimeenv.EngineWebKit} {
		t.Run(string(engine), func(t *testing.T) {
			testCopyObjectLargerThanBuffer(t, runtimeenv.Environment{Engine: engine}.Enabled(runtimeenv.BatchCopyExistence))
		})
	}
}

// testCopyObjectLargerThanBuffer verifies capacity bypass under both copy policies.
func testCopyObjectLargerThanBuffer(t *testing.T, batchExistence bool) {
	// Store an encoded block larger than the copy's pending-byte budget.
	ctx := t.Context()
	source := block_mock.NewMockStore(0)
	destination := block_mock.NewMockStore(0)
	data := make([]byte, 5<<20)
	root, _, err := source.PutBlock(ctx, data, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Copy between distinct buckets using the raw block constructor.
	src := NewCursor(ctx, nil, nil, nil, source, nil,
		&bucket.ObjectRef{BucketId: "source", RootRef: root},
		&bucket.BucketOpArgs{BucketId: "source"}, nil)
	defer src.Release()
	dest := NewCursor(ctx, nil, nil, nil, destination, nil,
		&bucket.ObjectRef{BucketId: "destination"},
		&bucket.BucketOpArgs{BucketId: "destination"}, nil)
	defer dest.Release()
	ref, stats, err := copyObjectToBucket(ctx, dest, src, byteslice.NewByteSliceBlock, 1, false, nil, nil, batchExistence)
	if err != nil {
		t.Fatal(err)
	}

	if stats.BlocksCopied != 1 || stats.BlocksWritten != 1 {
		t.Fatalf("large copy accounting: %+v", stats)
	}

	// A completed root must resolve to the entire original payload.
	stored, found, err := destination.GetBlock(ctx, ref.GetRootRef())
	if err != nil || !found || len(stored) != len(data) {
		t.Fatalf("large copy: bytes=%d found=%t err=%v", len(stored), found, err)
	}
}

// PutBlockBatch records each batch before passing it to storage.
func (d *copyDestination) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	d.batchSizes = append(d.batchSizes, len(entries))
	if d.batchErr != nil {
		return d.batchErr
	}
	return d.inner.PutBlockBatch(ctx, entries)
}

// Sync can fail the final fence independently of successful block writes.
func (d *copyDestination) Sync(ctx context.Context) (bool, error) {
	d.syncs++
	if d.syncErr != nil {
		return false, d.syncErr
	}
	return d.inner.Sync(ctx)
}

// sharedCopyBlock references the same child twice to form a shared subtree.
type sharedCopyBlock struct {
	bucket_mock.Root
}

// GetBlockRefs exposes two edges to the encoded child.
func (b *sharedCopyBlock) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	ref := b.GetExamplePtr().GetRootRef()
	return map[uint32]*block.BlockRef{1: ref, 2: ref}, nil
}

// GetBlockRefCtor decodes every child with the same shared-edge representation.
func (b *sharedCopyBlock) GetBlockRefCtor(uint32) block.Ctor {
	return func() block.Block { return &sharedCopyBlock{} }
}

// TestCopyObjectPrunesSharedSubtrees checks bounded traversal and complete storage.
func TestCopyObjectPrunesSharedSubtrees(t *testing.T) {
	for _, engine := range []runtimeenv.Engine{runtimeenv.EngineChromium, runtimeenv.EngineWebKit, runtimeenv.EngineUnknown} {
		t.Run(string(engine), func(t *testing.T) {
			testCopyObjectPrunesSharedSubtrees(t, runtimeenv.Environment{Engine: engine}.Enabled(runtimeenv.BatchCopyExistence))
		})
	}
}

// testCopyObjectPrunesSharedSubtrees exercises each policy against the same storage contract.
func testCopyObjectPrunesSharedSubtrees(t *testing.T, batchExistence bool) {
	// Build a graph with two references to each successive child.
	const depth = 8
	ctx := t.Context()
	source := block_mock.NewMockStore(0)
	destination := &copyDestination{inner: block_mock.NewMockStore(0)}
	refs := make([]*block.BlockRef, 0, depth+1)
	var root *block.BlockRef
	for range depth + 1 {
		blk := &sharedCopyBlock{
			ExamplePtr: &bucket.ObjectRef{BucketId: "child", RootRef: root},
		}
		data, err := blk.MarshalBlock()
		if err != nil {
			t.Fatal(err)
		}
		root, _, err = source.PutBlock(ctx, data, nil)
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, root)
	}

	// Give the copy distinct source and destination bucket identities.
	src := NewCursor(ctx, nil, nil, nil, source, nil,
		&bucket.ObjectRef{BucketId: "source", RootRef: root},
		&bucket.BucketOpArgs{BucketId: "source"}, nil)
	defer src.Release()
	dest := NewCursor(ctx, nil, nil, nil, destination, nil,
		&bucket.ObjectRef{BucketId: "destination"},
		&bucket.BucketOpArgs{BucketId: "destination"}, nil)
	defer dest.Release()

	// Count logical visits while concurrent workers prune shared descendants.
	var visits atomic.Int64
	var snapshots []ObjectCopyStats
	ref, stats, err := copyObjectToBucket(ctx, dest, src,
		func() block.Block { return &sharedCopyBlock{} }, 2, false,
		func(ent *WalkObjectBlocksEntry) (bool, error) {
			if !ent.Ref.GetEmpty() {
				visits.Add(1)
			}
			return true, ent.Err
		}, func(stats ObjectCopyStats) error {
			snapshots = append(snapshots, stats)
			return nil
		}, batchExistence)
	if err != nil {
		t.Fatal(err)
	}

	// The final progress snapshot includes the last drained batch.
	if len(snapshots) == 0 || snapshots[len(snapshots)-1] != stats {
		t.Fatalf("final progress does not match copy accounting: snapshots=%v stats=%+v", snapshots, stats)
	}
	for i := 1; i < len(snapshots); i++ {
		before, after := snapshots[i-1], snapshots[i]
		if after.BlocksSeen < before.BlocksSeen || after.BlocksCopied < before.BlocksCopied || after.BlocksExisting < before.BlocksExisting || after.BlocksWritten < before.BlocksWritten {
			t.Fatalf("progress moved backwards: before=%+v after=%+v", before, after)
		}
	}

	// Verify every distinct block reached a single fenced destination batch.
	if got, want := visits.Load(), int64(2*depth+1); got != want {
		t.Fatalf("visited %d blocks, want %d: shared descendants were revisited", got, want)
	}
	if stats.BlocksCopied != depth+1 || stats.BlocksDeduped != depth {
		t.Fatalf("unexpected copy accounting: %+v", stats)
	}
	if ref.GetBucketId() != "destination" || !ref.GetRootRef().EqualsRef(root) {
		t.Fatalf("unexpected destination ref: %v", ref)
	}
	for _, ref := range refs {
		_, found, err := destination.GetBlock(ctx, ref)
		if err != nil || !found {
			t.Fatalf("copied block %v: found=%v, err=%v", ref, found, err)
		}
	}
	if len(destination.batchSizes) != 1 || destination.batchSizes[0] != depth+1 || destination.syncs != 1 {
		t.Fatalf("copy did not batch and fence its writes: batches=%v syncs=%d", destination.batchSizes, destination.syncs)
	}
	wantSingle, wantBatch := int64(depth+1), int64(0)
	if batchExistence {
		wantSingle, wantBatch = 0, 1
	}
	if destination.singleProbes.Load() != wantSingle || destination.batchProbes.Load() != wantBatch || stats.BlocksWritten != depth+1 || stats.BlocksExisting != 0 {
		t.Fatalf("copy did not batch existence accounting: single=%d batch=%d stats=%+v", destination.singleProbes.Load(), destination.batchProbes.Load(), stats)
	}

	// Existing-subtree pruning still observes the root before walking children.
	_, skipped, err := copyObjectToBucket(ctx, dest, src,
		func() block.Block { return &sharedCopyBlock{} }, 2, true, nil, nil, batchExistence)
	if err != nil || skipped.BlocksCopied != 1 || skipped.BlocksExisting != 1 || skipped.SubtreesSkipped != 1 || destination.singleProbes.Load() != wantSingle+1 || destination.batchProbes.Load() != wantBatch {
		t.Fatalf("existing-subtree pruning changed: stats=%+v err=%v", skipped, err)
	}

	// Existing payloads still need the destination's ownership write. A failed
	// final batch or durability fence must never return a completed root.
	for _, failure := range []string{"batch", "sync"} {
		t.Run(failure, func(t *testing.T) {
			injected := errors.New("destination " + failure + " failed")
			destination.batchErr, destination.syncErr = nil, nil
			if failure == "batch" {
				destination.batchErr = injected
			} else {
				destination.syncErr = injected
			}
			ref, stats, err := copyObjectToBucket(ctx, dest, src,
				func() block.Block { return &sharedCopyBlock{} }, 2, false, nil, nil, batchExistence)
			if !errors.Is(err, injected) || ref != nil || stats.BlocksExisting != depth+1 {
				t.Fatalf("copy accepted failed destination: ref=%v stats=%+v err=%v", ref, stats, err)
			}
		})
	}

	// A progress consumer can abort even when accounting resolves at final drain.
	destination.batchErr, destination.syncErr = nil, nil
	syncs := destination.syncs
	ref, _, err = copyObjectToBucket(ctx, dest, src,
		func() block.Block { return &sharedCopyBlock{} }, 2, false, nil,
		func(stats ObjectCopyStats) error {
			if stats.BlocksCopied > 0 {
				return context.Canceled
			}
			return nil
		}, batchExistence)
	if !errors.Is(err, context.Canceled) || ref != nil || destination.syncs != syncs {
		t.Fatalf("copy accepted canceled progress: ref=%v syncs=%d err=%v", ref, destination.syncs, err)
	}
}

// GetHashType returns the destination hash type.
func (d *copyDestination) GetHashType() hash.HashType {
	return d.inner.GetHashType()
}

// GetSupportedFeatures returns the destination storage features.
func (d *copyDestination) GetSupportedFeatures() block.StoreFeature {
	return d.inner.GetSupportedFeatures()
}

// BeginReadOperation opens a destination read scope.
func (d *copyDestination) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	return d.inner.BeginReadOperation(ctx)
}

// PutBlock forwards individual writes without batch accounting.
func (d *copyDestination) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	return d.inner.PutBlock(ctx, data, opts)
}

// GetBlock reads a destination block.
func (d *copyDestination) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	return d.inner.GetBlock(ctx, ref)
}

// GetStoredBlock serves the block without refs because this test store
// keeps block bytes without their refs.
func (d *copyDestination) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	return block.GetBlockWithoutRefs(ctx, d, ref)
}

// RmBlock removes a destination block.
func (d *copyDestination) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	return d.inner.RmBlock(ctx, ref)
}

// StatBlock reads destination metadata.
func (d *copyDestination) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	return d.inner.StatBlock(ctx, ref)
}
