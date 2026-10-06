package provider_local

import (
	"context"
	"slices"

	"github.com/s4wave/spacewave/core/sobject"
)

// DeleteSharedObject deletes the shared object with the given id.
func (a *ProviderAccount) DeleteSharedObject(ctx context.Context, id string) error {
	// Publish the deletion first so interrupted cleanup cannot resurrect it.
	var entry *sobject.SharedObjectListEntry
	for _, candidate := range a.soListCtr.GetValue().GetSharedObjects() {
		if candidate.GetRef().GetProviderResourceRef().GetId() == id {
			entry = candidate
			break
		}
	}
	if entry == nil {
		return sobject.ErrSharedObjectNotFound
	}
	if err := a.publishAccountCatalogEntry(ctx, entry, true); err != nil {
		return err
	}

	// Local cleanup follows the committed account decision.
	relMtx, err := a.mtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer relMtx()

	// Delete the shared object unless another cleanup already removed it.
	if err := a.deleteSharedObjectLocked(ctx, id); err != nil && err != sobject.ErrSharedObjectNotFound {
		return err
	}
	return nil
}

// deleteSharedObjectLocked deletes a shared object. Assumes mtx is locked.
func (a *ProviderAccount) deleteSharedObjectLocked(ctx context.Context, id string) error {
	// Read the provider and account that own the shared-object bucket.
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

	// Resolve the block store and bucket for the shared object being removed.
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

	// Drop the deleted bucket's GC refs and sweep newly rootless blocks.
	a.removeSharedObjectGCRefs(ctx, providerID, bucketID, a.le.WithField("sobject-id", id))
	a.triggerGCCleanup()

	return nil
}
