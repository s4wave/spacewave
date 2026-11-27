package resource_session

import (
	"context"
	"time"

	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/session"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// CreateLinkedLocalSession creates a local provider session with cloud identity metadata.
func (r *SpacewaveSessionResource) CreateLinkedLocalSession(
	ctx context.Context,
	req *s4wave_provider_spacewave.CreateLinkedLocalSessionRequest,
) (*s4wave_provider_spacewave.CreateLinkedLocalSessionResponse, error) {
	// Open the Session controller used to create or find the linked local Session.
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, r.b, "", false, nil)
	if err != nil {
		return nil, err
	}
	defer sessionCtrlRef.Release()

	// Scope the idempotency check to the Cloud Session receiving the link.
	sessionID := r.getSessionID()

	// Idempotency: check if a linked local session already exists.
	found, localIdx, err := r.swAcc.GetLinkedLocalSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if found {
		entry, err := sessionCtrl.GetSessionByIdx(ctx, localIdx)
		if err != nil {
			return nil, err
		}
		return &s4wave_provider_spacewave.CreateLinkedLocalSessionResponse{SessionListEntry: entry}, nil
	}

	// Read the Cloud account identity used for the new local Session.
	info, err := r.swAcc.GetAccountState(ctx)
	if err != nil {
		return nil, err
	}

	// Resolve the local provider that will create the linked Session.
	localProv, localProvRef, err := provider.ExLookupProvider(ctx, r.b, provider_local.ProviderID, false, nil)
	if err != nil {
		return nil, err
	}
	defer localProvRef.Release()

	// Create a local account and Session for the Cloud identity.
	prov := localProv.(*provider_local.Provider)
	localSessRef, err := prov.CreateLocalAccountAndSession(ctx, info.AccountId)
	if err != nil {
		return nil, err
	}

	// Build local Session metadata with the linked Cloud account identity.
	meta := &session.SessionMetadata{
		DisplayName:         info.EntityId,
		ProviderDisplayName: "Local",
		ProviderAccountId:   localSessRef.GetProviderResourceRef().GetProviderAccountId(),
		CloudAccountId:      info.AccountId,
		CloudEntityId:       info.EntityId,
		ProviderId:          "local",
		CreatedAt:           time.Now().UnixMilli(),
	}
	listEntry, err := sessionCtrl.RegisterSession(ctx, localSessRef, meta)
	if err != nil {
		return nil, err
	}

	// Write linked-local cross-reference on the spacewave sessionTracker ObjectStore.
	if err := r.swAcc.SetLinkedLocalSession(ctx, sessionID, listEntry.GetSessionIndex()); err != nil {
		r.le.WithError(err).Warn("failed to write linked-local key")
	}

	return &s4wave_provider_spacewave.CreateLinkedLocalSessionResponse{SessionListEntry: listEntry}, nil
}

// GetLinkedLocalSession returns the session index of the linked local session.
func (r *SpacewaveSessionResource) GetLinkedLocalSession(
	ctx context.Context,
	req *s4wave_provider_spacewave.GetLinkedLocalSessionRequest,
) (*s4wave_provider_spacewave.GetLinkedLocalSessionResponse, error) {
	sessionID := r.getSessionID()
	found, localIdx, err := r.swAcc.GetLinkedLocalSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	return &s4wave_provider_spacewave.GetLinkedLocalSessionResponse{
		Found:        found,
		SessionIndex: localIdx,
	}, nil
}

// UnlinkLocalSession removes the linked-local session cross-reference.
func (r *SpacewaveSessionResource) UnlinkLocalSession(
	ctx context.Context,
	req *s4wave_provider_spacewave.UnlinkLocalSessionRequest,
) (*s4wave_provider_spacewave.UnlinkLocalSessionResponse, error) {
	sessionID := r.getSessionID()

	if err := r.swAcc.DeleteLinkedLocalSession(ctx, sessionID); err != nil {
		return nil, err
	}

	r.swAcc.GetAccountBroadcast().HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		broadcast()
	})

	return &s4wave_provider_spacewave.UnlinkLocalSessionResponse{}, nil
}
