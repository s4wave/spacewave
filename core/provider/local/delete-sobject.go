package provider_local

import (
	"context"
	"slices"

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

	a.removeSharedObjectGCRefs(ctx, providerID, bucketID, a.le.WithField("sobject-id", id))
	a.triggerGCCleanup()

	return nil
}
