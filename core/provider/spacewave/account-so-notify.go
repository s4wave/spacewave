package provider_spacewave

import api "github.com/s4wave/spacewave/core/provider/spacewave/api"

func (a *ProviderAccount) handleAccountSONotify(
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
		return
	}
	if payload.GetChangeType() == "metadata" && payload.GetMetadata() != nil {
		a.SetSharedObjectMetadata(soID, payload.GetMetadata())
		a.PatchSharedObjectListMetadata(soID, payload.GetMetadata())
	}
	if a.HasCachedSharedObject(soID) {
		return
	}
	a.le.WithField("sobject-id", soID).Debug("invalidating shared object list after so notify for unknown so")
	a.invalidateSharedObjectList()
}
