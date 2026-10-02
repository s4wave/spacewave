package provider_spacewave

import (
	"context"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

// handleAccountSONotify applies an account-level shared object notify. A
// delete drops the object, metadata patches the cached metadata, and an access
// change refetches the shared object list. Other notifies refetch the list only
// when the object is unknown.
func (a *ProviderAccount) handleAccountSONotify(
	ctx context.Context,
	soID string,
	payload *api.SONotifyEventPayload,
) {
	// A notify without an object has nothing to apply.
	if soID == "" {
		return
	}

	// A deleted object leaves the account entirely.
	if payload.GetChangeType() == "delete" {
		a.DeleteSharedObjectMetadata(soID)
		a.RemoveSharedObjectListEntry(soID)
		if a.sobjects != nil {
			a.sobjects.RemoveKey(soID)
		}
		a.removeSharedObjectGCRefs(ctx, soID, a.le.WithField("sobject-id", soID))
		a.triggerGCCleanup()
		return
	}

	// Patch cached metadata in place.
	if payload.GetChangeType() == "metadata" && payload.GetMetadata() != nil {
		a.SetSharedObjectMetadata(soID, payload.GetMetadata())
		a.PatchSharedObjectListMetadata(soID, payload.GetMetadata())
	}

	// The cloud list now decides membership, even for an object this account
	// created.
	if payload.GetChangeType() == "access_changed" {
		a.settleCreatedSharedObjectListEntry(soID)
		a.le.WithField("sobject-id", soID).Debug("invalidating shared object list after access change")
		a.invalidateSharedObjectList()
		return
	}

	// Refetch the list for an object the account does not know yet.
	if a.HasCachedSharedObject(soID) {
		return
	}
	a.le.WithField("sobject-id", soID).Debug("invalidating shared object list after so notify for unknown so")
	a.invalidateSharedObjectList()
}
