package resource_session

import (
	"context"

	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	"github.com/s4wave/spacewave/core/resource/session/sessioninfo"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/volume"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// WatchOnboardingStatus streams Onboarding Status, the Spacewave cloud session
// route-status projection used by root, plan, billing, and lifecycle routers.
func (r *SpacewaveSessionResource) WatchOnboardingStatus(
	req *s4wave_provider_spacewave.WatchOnboardingStatusRequest,
	strm s4wave_session.SRPCSpacewaveSessionResourceService_WatchOnboardingStatusStream,
) error {
	// Initialize the session projection watch state.
	ctx := strm.Context()
	sessionID := r.getSessionID()
	sessRef := r.getSessionRef()
	accountBcast := r.swAcc.GetAccountBroadcast()

	// Retain the prior route projection so unchanged account state is not resent.
	var prev *s4wave_provider_spacewave.WatchOnboardingStatusResponse

	// Read account status and wait channel for each update.
	for {
		var ch <-chan struct{}
		var accountStatus provider.ProviderAccountStatus
		var stateLoaded bool
		accountBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
			accountStatus = r.swAcc.GetAccountStatus()
			state := r.swAcc.AccountStateSnapshot()
			if state != nil {
				stateLoaded = true
			}
		})

		// Hold the first route-status projection until the account snapshot
		// has been populated from the cloud, unless the account is in a
		// terminal non-ready state. Prevents a "not subscribed" flash for
		// subscribed users while the account fetcher is still loading.
		if prev == nil && !sessioninfo.ShouldEmitOnboardingStatus(stateLoaded, accountStatus) {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ch:
			}
			continue
		}

		// Build the current onboarding projection.
		projCtx := r.buildOnboardingStatusProjectionContext(ctx, sessionID, sessRef)
		resp, err := r.swAcc.BuildOnboardingStatusProjection(ctx, projCtx)
		if err != nil {
			r.le.WithError(err).Warn("failed to fetch managed billing account list")
		}

		// Emit only a changed projection.
		if prev == nil || !resp.EqualVT(prev) {
			if err := strm.Send(resp); err != nil {
				return err
			}
			prev = resp
		}

		// Wait for account changes or cancellation.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}

// buildOnboardingStatusProjectionContext collects linked-session inputs for the account projection.
func (r *SpacewaveSessionResource) buildOnboardingStatusProjectionContext(
	ctx context.Context,
	sessionID string,
	sessRef *session.SessionRef,
) provider_spacewave.OnboardingStatusProjectionContext {
	// Collect linked-session indicators for the onboarding route projection.
	var projCtx provider_spacewave.OnboardingStatusProjectionContext

	// Resolve linked local-session state.
	found, localIdx, _ := r.swAcc.GetLinkedLocalSession(ctx, sessionID)
	if found {
		projCtx.HasLinkedLocal = true
		projCtx.LinkedLocalSessionIndex = localIdx

		// Resolve the linked cloud session for local sessions.
		projCtx.LinkedLocalHasContent = r.checkLocalHasContent(ctx, localIdx)
	}

	// Resolve the linked cloud session for local sessions that need migration.
	if sessRef.GetProviderResourceRef().GetProviderId() != "spacewave" {
		cloudIdx := r.findCloudSessionIndex(ctx, r.swAcc.GetAccountID())
		if cloudIdx != 0 {
			projCtx.HasLinkedCloud = true
			projCtx.LinkedCloudSessionIndex = cloudIdx
		}
	}
	return projCtx
}

// checkLocalHasContent returns true if the local session at the given index has SharedObjects.
func (r *SpacewaveSessionResource) checkLocalHasContent(ctx context.Context, localIdx uint32) bool {
	// Open the Session controller needed to inspect the local account.
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, r.b, "", false, nil)
	if err != nil {
		return false
	}
	defer sessionCtrlRef.Release()

	// Resolve the local Session entry selected by the linked-session index.
	entry, err := sessionCtrl.GetSessionByIdx(ctx, localIdx)
	if err != nil {
		return false
	}
	provRef := entry.GetSessionRef().GetProviderResourceRef()
	providerID := provRef.GetProviderId()
	accountID := provRef.GetProviderAccountId()

	// Open the provider account that owns the local Session's shared-object list.
	provAcc, provAccRef, accErr := provider.ExAccessProviderAccount(ctx, r.b, providerID, accountID, false, nil)
	if accErr != nil {
		return false
	}
	defer provAccRef.Release()
	localAcc, ok := provAcc.(*provider_local.ProviderAccount)
	if !ok {
		return false
	}

	// Resolve the local volume and ObjectStore used to inspect shared objects.
	volID := localAcc.GetVolume().GetID()
	soObjStoreID := provider_local.SobjectObjectStoreID(providerID, accountID)
	soHandle, _, soDiRef, soErr := volume.ExBuildObjectStoreAPI(ctx, r.b, false, soObjStoreID, volID, nil)
	if soErr != nil {
		return false
	}
	defer soDiRef.Release()

	// Use the local provider ObjectStore to read the shared-object list.
	soStore := soHandle.GetObjectStore()
	soTx, txErr := soStore.NewTransaction(ctx, false)
	if txErr != nil {
		return false
	}
	defer soTx.Discard()

	// Read the local Session's shared-object list record.
	data, found, gErr := soTx.Get(ctx, provider_local.SobjectObjectStoreListKey())
	if gErr != nil || !found {
		return false
	}
	list := &sobject.SharedObjectList{}
	if err := list.UnmarshalVT(data); err != nil {
		return false
	}
	return len(list.GetSharedObjects()) > 0
}

// findCloudSessionIndex finds the session index for a spacewave (cloud) session
// with the given account ID. Returns 0 if not found.
func (r *SpacewaveSessionResource) findCloudSessionIndex(ctx context.Context, accountID string) uint32 {
	// Open the Session controller before searching for the Cloud account.
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, r.b, "", false, nil)
	if err != nil {
		return 0
	}
	defer sessionCtrlRef.Release()

	// Read the Session entries and select the matching Cloud account.
	sessions, err := sessionCtrl.ListSessions(ctx)
	if err != nil {
		return 0
	}
	for _, entry := range sessions {
		ref := entry.GetSessionRef().GetProviderResourceRef()
		if ref.GetProviderAccountId() == accountID && ref.GetProviderId() == "spacewave" {
			return entry.GetSessionIndex()
		}
	}
	return 0
}
