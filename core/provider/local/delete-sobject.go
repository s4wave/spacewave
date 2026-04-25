package provider_local

import (
	"context"
	"slices"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
	kvtx_volume "github.com/s4wave/spacewave/db/volume/common/kvtx"

	"github.com/s4wave/spacewave/core/sobject"
)

// DeleteSharedObject deletes the shared object with the given id.
func (a *ProviderAccount) DeleteSharedObject(ctx context.Context, id string) error {
	relMtx, err := a.mtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer relMtx()

	return a.deleteSharedObjectLocked(ctx, id)
}

// deleteSharedObjectLocked deletes a shared object. Assumes mtx is locked.
func (a *ProviderAccount) deleteSharedObjectLocked(ctx context.Context, id string) error {
	providerID := a.t.accountInfo.GetProviderId()
	providerAccountID := a.t.accountInfo.GetProviderAccountId()

	// Get current list
	sharedObjectList := a.soListCtr.GetValue().CloneVT()
	if sharedObjectList == nil {
		return sobject.ErrSharedObjectNotFound
	}

	// Find and remove the shared object from the list
	idx := slices.IndexFunc(sharedObjectList.GetSharedObjects(), func(e *sobject.SharedObjectListEntry) bool {
		return e.GetRef().GetProviderResourceRef().GetId() == id
	})
	if idx == -1 {
		return sobject.ErrSharedObjectNotFound
	}

	soEntry := sharedObjectList.GetSharedObjects()[idx]
	blockStoreID := soEntry.GetRef().GetBlockStoreId()
	bucketID := BlockStoreBucketID(providerID, providerAccountID, blockStoreID)

	// Remove from list
	sharedObjectList.SharedObjects = slices.Delete(sharedObjectList.SharedObjects, idx, idx+1)

	// Write updated list
	if err := a.writeSharedObjectList(ctx, sharedObjectList); err != nil {
		return err
	}
	a.soListCtr.SetValue(sharedObjectList)

	// Remove GC edges: provider -> bucket (marks orphans as unreferenced)
	if kvVol, ok := a.vol.(kvtx_volume.KvtxVolume); ok {
		if rg := kvVol.GetRefGraph(); rg != nil {
			gcOps := block_gc.NewGCStoreOps(a.vol, rg)
			bucketIRI := block_gc.BucketIRI(bucketID)
			if err := gcOps.RemoveGCRef(ctx, block_gc.NodeGCRoot, bucketIRI); err != nil {
				a.le.WithError(err).Warn("failed to remove gc root ref for deleted sobject bucket")
			}
			if err := gcOps.RemoveGCRef(ctx, ProviderIRI(providerID), block_gc.BucketIRI(bucketID)); err != nil {
				a.le.WithError(err).Warn("failed to remove gc ref for deleted sobject")
			}

			// Run immediate collection
			collector := block_gc.NewCollector(rg, a.vol, nil)
			if _, err := collector.Collect(ctx); err != nil {
				a.le.WithError(err).Warn("gc collect after delete failed")
			}
		}
	}

	return nil
}
