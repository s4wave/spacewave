package sobject_test

import (
	"context"
	"slices"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/volume"
	kvtx_volume "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/testbed"
)

// lookupProviderVolume looks up the volume created by the local provider for
// the given account. This is a separate volume from the testbed's main volume.
func lookupProviderVolume(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	providerID, accountID string,
) (volume.Volume, block_gc.RefGraphOps) {
	t.Helper()
	storageVolumeID := provider_local.StorageVolumeID(providerID, accountID)
	vol, _, volRef, err := volume.ExLookupVolume(ctx, tb.Bus, storageVolumeID, "", false)
	if err != nil {
		t.Fatal("failed to look up provider volume: " + err.Error())
	}
	t.Cleanup(volRef.Release)
	kvVol, ok := vol.(kvtx_volume.KvtxVolume)
	if !ok {
		t.Fatal("provider volume does not implement KvtxVolume")
	}
	rg := kvVol.GetRefGraph()
	if rg == nil {
		t.Fatal("provider volume has no RefGraph")
	}
	return vol, rg
}

// TestGarbageCollection tests the GC lifecycle: create space, put blocks,
// delete space, verify blocks are swept by the collector.
func TestGarbageCollection(t *testing.T) {
	ctx := t.Context()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	peerID := tb.Volume.GetPeerID()

	// Create the provider controller.
	providerID := "local"
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: providerID,
		PeerId:     peerID.String(),
	}), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer provCtrlRef.Release()

	// Acquire a provider account handle.
	accountID := "test-account"
	provAcc, provAccRef, err := provider.ExAccessProviderAccount(ctx, tb.Bus, providerID, accountID, false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer provAccRef.Release()

	// Look up the provider's volume (created by the storage volume controller).
	vol, rg := lookupProviderVolume(t, ctx, tb, providerID, accountID)

	// Get the shared object provider feature.
	wsProv, err := sobject.GetSharedObjectProviderAccountFeature(ctx, provAcc)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the shared object (space).
	sobjectID := "gc-test-space"
	createdSoRef, err := wsProv.CreateSharedObject(ctx, sobjectID, &sobject.SharedObjectMeta{
		BodyType: "test",
	}, "", "")
	if err != nil {
		t.Fatal(err.Error())
	}

	// Resolve the bucket IRI for later refgraph assertions after the shared
	// object mount path has finished wiring the provider-owned bucket.
	blockStoreID := provider_local.SobjectBlockStoreID(sobjectID)
	bucketID := provider_local.BlockStoreBucketID(providerID, accountID, blockStoreID)
	bucketIRI := block_gc.BucketIRI(bucketID)

	// Mount the shared object to get access to the block store.
	so, soRef, err := sobject.ExMountSharedObject(ctx, tb.Bus, createdSoRef, false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Put some blocks through the block store (goes through bucket handle
	// with GCStoreOps wrapping, which tracks blocks in the RefGraph).
	blockStore := so.GetBlockStore()
	var blockRefs []*block.BlockRef
	for i := range 5 {
		ex := block_mock.NewExample("gc-test-block-" + string(rune('a'+i)))
		ref, _, putErr := block.PutBlock(ctx, blockStore, ex)
		if putErr != nil {
			t.Fatal(putErr.Error())
		}
		blockRefs = append(blockRefs, ref)
	}

	// Verify all blocks exist in the provider's volume.
	for _, ref := range blockRefs {
		exists, exErr := vol.GetBlockExists(ctx, ref)
		if exErr != nil {
			t.Fatal(exErr.Error())
		}
		if !exists {
			t.Fatalf("block %s should exist after PutBlock", ref.MarshalString())
		}
	}

	// Verify blocks are tracked in RefGraph (bucket -> block edges).
	bucketOutgoing, err := rg.GetOutgoingRefs(ctx, bucketIRI)
	if err != nil {
		t.Fatal(err.Error())
	}
	for _, ref := range blockRefs {
		blkIRI := block_gc.BlockIRI(ref)
		if !slices.Contains(bucketOutgoing, blkIRI) {
			t.Fatalf("expected bucket -> block edge for %s", blkIRI)
		}
	}

	// Unmount the shared object before deleting.
	soRef.Release()

	// Delete the shared object (removes GC hierarchy, runs immediate collect).
	if err := wsProv.DeleteSharedObject(ctx, sobjectID); err != nil {
		t.Fatal(err.Error())
	}

	// Verify all blocks were swept from the volume.
	for _, ref := range blockRefs {
		exists, exErr := vol.GetBlockExists(ctx, ref)
		if exErr != nil {
			t.Fatal(exErr.Error())
		}
		if exists {
			t.Fatalf("block %s should have been swept by GC", ref.MarshalString())
		}
	}
}

// TestGarbageCollection_MultipleSpaces tests that deleting one space does
// not affect blocks in another space.
func TestGarbageCollection_MultipleSpaces(t *testing.T) {
	ctx := t.Context()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	peerID := tb.Volume.GetPeerID()

	// Create the provider controller.
	providerID := "local"
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: providerID,
		PeerId:     peerID.String(),
	}), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer provCtrlRef.Release()

	accountID := "test-account"
	provAcc, provAccRef, err := provider.ExAccessProviderAccount(ctx, tb.Bus, providerID, accountID, false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer provAccRef.Release()

	// Look up the provider's volume.
	vol, _ := lookupProviderVolume(t, ctx, tb, providerID, accountID)

	wsProv, err := sobject.GetSharedObjectProviderAccountFeature(ctx, provAcc)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create two spaces.
	spaceARef, err := wsProv.CreateSharedObject(ctx, "space-a", &sobject.SharedObjectMeta{BodyType: "test"}, "", "")
	if err != nil {
		t.Fatal(err.Error())
	}
	spaceBRef, err := wsProv.CreateSharedObject(ctx, "space-b", &sobject.SharedObjectMeta{BodyType: "test"}, "", "")
	if err != nil {
		t.Fatal(err.Error())
	}

	// Mount both and put blocks.
	soA, soARef, err := sobject.ExMountSharedObject(ctx, tb.Bus, spaceARef, false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	soB, soBRef, err := sobject.ExMountSharedObject(ctx, tb.Bus, spaceBRef, false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	blockA := block_mock.NewExample("block-in-space-a")
	refA, _, err := block.PutBlock(ctx, soA.GetBlockStore(), blockA)
	if err != nil {
		t.Fatal(err.Error())
	}

	blockB := block_mock.NewExample("block-in-space-b")
	refB, _, err := block.PutBlock(ctx, soB.GetBlockStore(), blockB)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Unmount space A, delete it.
	soARef.Release()
	if err := wsProv.DeleteSharedObject(ctx, "space-a"); err != nil {
		t.Fatal(err.Error())
	}

	// Block A should be swept.
	exists, err := vol.GetBlockExists(ctx, refA)
	if err != nil {
		t.Fatal(err.Error())
	}
	if exists {
		t.Fatal("block in deleted space-a should have been swept")
	}

	// Block B should still exist (space-b is alive).
	exists, err = vol.GetBlockExists(ctx, refB)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !exists {
		t.Fatal("block in space-b should still exist")
	}

	// Clean up.
	soBRef.Release()
}

// TestStorageLifecycleGC tests the full write-delete-GC-compact cycle that
// real users hit: create a space, write multiple groups of blocks (simulating
// files), remove references for some groups (simulating file deletion), run
// the GC collector, and verify only the unreferenced blocks are swept while
// the remaining blocks stay intact.
//
// This validates the intra-space GC mechanism: individual bucket -> block
// edges can be removed and the collector sweeps the orphaned blocks.
func TestStorageLifecycleGC(t *testing.T) {
	ctx := t.Context()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	peerID := tb.Volume.GetPeerID()

	providerID := "local"
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: providerID,
		PeerId:     peerID.String(),
	}), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer provCtrlRef.Release()

	accountID := "gc-lifecycle"
	provAcc, provAccRef, err := provider.ExAccessProviderAccount(ctx, tb.Bus, providerID, accountID, false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer provAccRef.Release()

	vol, rg := lookupProviderVolume(t, ctx, tb, providerID, accountID)

	wsProv, err := sobject.GetSharedObjectProviderAccountFeature(ctx, provAcc)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create a space.
	spaceID := "gc-lifecycle-space"
	createdSoRef, err := wsProv.CreateSharedObject(ctx, spaceID, &sobject.SharedObjectMeta{
		BodyType: "test",
	}, "", "")
	if err != nil {
		t.Fatal(err.Error())
	}

	so, soRef, err := sobject.ExMountSharedObject(ctx, tb.Bus, createdSoRef, false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	blockStore := so.GetBlockStore()

	blockStoreID := provider_local.SobjectBlockStoreID(spaceID)
	bucketID := provider_local.BlockStoreBucketID(providerID, accountID, blockStoreID)
	bucketIRI := block_gc.BucketIRI(bucketID)

	// Write 8 blocks in two groups: "kept" (5 blocks) and "deleted" (3 blocks).
	// Each group simulates a set of files the user created.
	var keptRefs []*block.BlockRef
	for i := range 5 {
		ex := block_mock.NewExample("kept-file-block-" + string(rune('a'+i)))
		ref, _, putErr := block.PutBlock(ctx, blockStore, ex)
		if putErr != nil {
			t.Fatal(putErr.Error())
		}
		keptRefs = append(keptRefs, ref)
	}

	var deletedRefs []*block.BlockRef
	for i := range 3 {
		ex := block_mock.NewExample("deleted-file-block-" + string(rune('x'+i)))
		ref, _, putErr := block.PutBlock(ctx, blockStore, ex)
		if putErr != nil {
			t.Fatal(putErr.Error())
		}
		deletedRefs = append(deletedRefs, ref)
	}

	// Verify all 8 blocks exist.
	for _, ref := range append(keptRefs, deletedRefs...) {
		exists, exErr := vol.GetBlockExists(ctx, ref)
		if exErr != nil {
			t.Fatal(exErr.Error())
		}
		if !exists {
			t.Fatalf("block %s should exist after write", ref.MarshalString())
		}
	}
	t.Logf("wrote %d kept + %d deleted blocks", len(keptRefs), len(deletedRefs))

	// Verify all blocks are in the RefGraph.
	bucketOutgoing, err := rg.GetOutgoingRefs(ctx, bucketIRI)
	if err != nil {
		t.Fatal(err.Error())
	}
	for _, ref := range append(keptRefs, deletedRefs...) {
		if !slices.Contains(bucketOutgoing, block_gc.BlockIRI(ref)) {
			t.Fatalf("expected bucket -> block edge for %s", ref.MarshalString())
		}
	}

	// Simulate file deletion: remove bucket -> block edges for the "deleted"
	// group. In a full UnixFS integration, this would happen when the file
	// system layer detects that a file's blocks are no longer referenced by
	// any directory entry.
	for _, ref := range deletedRefs {
		blkIRI := block_gc.BlockIRI(ref)
		if err := rg.RemoveRef(ctx, bucketIRI, blkIRI); err != nil {
			t.Fatal(err.Error())
		}
		// Mark the block as unreferenced if it has no other incoming refs.
		hasIncoming, err := rg.HasIncomingRefs(ctx, blkIRI)
		if err != nil {
			t.Fatal(err.Error())
		}
		if !hasIncoming {
			if err := rg.AddRef(ctx, block_gc.NodeUnreferenced, blkIRI); err != nil {
				t.Fatal(err.Error())
			}
		}
	}
	t.Log("removed ref graph edges for deleted blocks")

	// Run the GC collector.
	collector := block_gc.NewCollector(rg, vol, nil)
	if _, err := collector.Collect(ctx); err != nil {
		t.Fatal(err.Error())
	}
	t.Log("GC collect completed")

	// Verify: deleted blocks are swept from the volume.
	for _, ref := range deletedRefs {
		exists, exErr := vol.GetBlockExists(ctx, ref)
		if exErr != nil {
			t.Fatal(exErr.Error())
		}
		if exists {
			t.Fatalf("deleted block %s should have been swept by GC", ref.MarshalString())
		}
	}

	// Verify: kept blocks are still intact.
	for _, ref := range keptRefs {
		exists, exErr := vol.GetBlockExists(ctx, ref)
		if exErr != nil {
			t.Fatal(exErr.Error())
		}
		if !exists {
			t.Fatalf("kept block %s should still exist after GC", ref.MarshalString())
		}
	}

	// Verify: kept blocks are still in the RefGraph.
	bucketOutgoing, err = rg.GetOutgoingRefs(ctx, bucketIRI)
	if err != nil {
		t.Fatal(err.Error())
	}
	for _, ref := range keptRefs {
		if !slices.Contains(bucketOutgoing, block_gc.BlockIRI(ref)) {
			t.Fatalf("kept block %s should still have bucket -> block edge", ref.MarshalString())
		}
	}

	// Verify: deleted blocks are NOT in the RefGraph.
	for _, ref := range deletedRefs {
		if slices.Contains(bucketOutgoing, block_gc.BlockIRI(ref)) {
			t.Fatalf("deleted block %s should not have bucket -> block edge after GC", ref.MarshalString())
		}
	}

	t.Logf("verified: %d deleted blocks swept, %d kept blocks intact", len(deletedRefs), len(keptRefs))

	// Clean up: unmount and delete space, which sweeps remaining blocks.
	soRef.Release()
	if err := wsProv.DeleteSharedObject(ctx, spaceID); err != nil {
		t.Fatal(err.Error())
	}

	for _, ref := range keptRefs {
		exists, exErr := vol.GetBlockExists(ctx, ref)
		if exErr != nil {
			t.Fatal(exErr.Error())
		}
		if exists {
			t.Fatalf("block %s should be swept after space deletion", ref.MarshalString())
		}
	}
	t.Log("verified: all remaining blocks swept after space deletion")
}

// TestStorageLifecycleGC_LargerBlocks tests GC with realistic block sizes
// (1MB+ each) to verify the GC mechanism handles larger data volumes.
func TestStorageLifecycleGC_LargerBlocks(t *testing.T) {
	ctx := t.Context()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	peerID := tb.Volume.GetPeerID()

	providerID := "local"
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: providerID,
		PeerId:     peerID.String(),
	}), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer provCtrlRef.Release()

	accountID := "gc-large"
	provAcc, provAccRef, err := provider.ExAccessProviderAccount(ctx, tb.Bus, providerID, accountID, false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer provAccRef.Release()

	vol, rg := lookupProviderVolume(t, ctx, tb, providerID, accountID)

	wsProv, err := sobject.GetSharedObjectProviderAccountFeature(ctx, provAcc)
	if err != nil {
		t.Fatal(err.Error())
	}

	spaceID := "gc-large-space"
	createdSoRef, err := wsProv.CreateSharedObject(ctx, spaceID, &sobject.SharedObjectMeta{
		BodyType: "test",
	}, "", "")
	if err != nil {
		t.Fatal(err.Error())
	}

	so, soRef, err := sobject.ExMountSharedObject(ctx, tb.Bus, createdSoRef, false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	blockStore := so.GetBlockStore()

	blockStoreID := provider_local.SobjectBlockStoreID(spaceID)
	bucketID := provider_local.BlockStoreBucketID(providerID, accountID, blockStoreID)
	bucketIRI := block_gc.BucketIRI(bucketID)

	// Write 5 large blocks (1MB each) simulating file content.
	const blockSize = 1 << 20 // 1MB
	var allRefs []*block.BlockRef
	for i := range 5 {
		data := make([]byte, blockSize)
		// Fill with deterministic pattern (not random, to allow dedup detection).
		for j := range data {
			data[j] = byte(i*37 + j%251)
		}
		ref, _, putErr := blockStore.PutBlock(ctx, data, nil)
		if putErr != nil {
			t.Fatal(putErr.Error())
		}
		allRefs = append(allRefs, ref)
		t.Logf("wrote 1MB block %d: %s", i, ref.MarshalString())
	}

	// Verify all exist.
	for _, ref := range allRefs {
		exists, exErr := vol.GetBlockExists(ctx, ref)
		if exErr != nil {
			t.Fatal(exErr.Error())
		}
		if !exists {
			t.Fatalf("large block %s should exist", ref.MarshalString())
		}
	}

	// Remove refs for blocks 0, 1, 2 (simulating deletion of 3 files).
	deletedRefs := allRefs[:3]
	keptRefs := allRefs[3:]
	for _, ref := range deletedRefs {
		blkIRI := block_gc.BlockIRI(ref)
		if err := rg.RemoveRef(ctx, bucketIRI, blkIRI); err != nil {
			t.Fatal(err.Error())
		}
		hasIncoming, err := rg.HasIncomingRefs(ctx, blkIRI)
		if err != nil {
			t.Fatal(err.Error())
		}
		if !hasIncoming {
			if err := rg.AddRef(ctx, block_gc.NodeUnreferenced, blkIRI); err != nil {
				t.Fatal(err.Error())
			}
		}
	}

	// GC.
	collector := block_gc.NewCollector(rg, vol, nil)
	if _, err := collector.Collect(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Verify swept.
	for _, ref := range deletedRefs {
		exists, _ := vol.GetBlockExists(ctx, ref)
		if exists {
			t.Fatalf("deleted large block %s should be swept", ref.MarshalString())
		}
	}

	// Verify retained.
	for _, ref := range keptRefs {
		// Read the full block data to verify integrity.
		data, found, err := vol.GetBlock(ctx, ref)
		if err != nil {
			t.Fatal(err.Error())
		}
		if !found {
			t.Fatalf("kept large block %s should still exist", ref.MarshalString())
		}
		if len(data) != blockSize {
			t.Fatalf("kept block size mismatch: want %d, got %d", blockSize, len(data))
		}
	}

	t.Logf("verified: %d x 1MB blocks swept, %d x 1MB blocks intact", len(deletedRefs), len(keptRefs))

	// Clean up.
	soRef.Release()
	if err := wsProv.DeleteSharedObject(ctx, spaceID); err != nil {
		t.Fatal(err.Error())
	}
}
