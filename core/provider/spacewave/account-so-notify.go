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
	if soID == "" {
		return
	}
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
	if payload.GetChangeType() == "metadata" && payload.GetMetadata() != nil {
		a.SetSharedObjectMetadata(soID, payload.GetMetadata())
		a.PatchSharedObjectListMetadata(soID, payload.GetMetadata())
	}
	if payload.GetChangeType() == "access_changed" {
		a.le.WithField("sobject-id", soID).Debug("invalidating shared object list after access change")
		a.invalidateSharedObjectList()
		return
	}
	if a.HasCachedSharedObject(soID) {
		return
	}
	a.le.WithField("sobject-id", soID).Debug("invalidating shared object list after so notify for unknown so")
	a.invalidateSharedObjectList()
}
