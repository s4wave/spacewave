package resource_root

import (
	"context"
	"slices"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/core/provider"
	resource_account "github.com/s4wave/spacewave/core/resource/account"
	resource_session "github.com/s4wave/spacewave/core/resource/session"
	"github.com/s4wave/spacewave/core/session"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
)

// MountSession mounts a session and returns the Session resource by SessionRef.
func (s *CoreRootServer) MountSession(
	ctx context.Context,
	req *s4wave_root.MountSessionRequest,
) (*s4wave_root.MountSessionResponse, error) {
	// Validate the request and find the caller's resource client.
	if err := req.GetSessionRef().Validate(); err != nil {
		return nil, err
	}
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Mount the Session.
	sess, sessRef, err := session.ExMountSession(ctx, s.b, req.GetSessionRef(), false, nil)
	if err != nil {
		return nil, err
	}

	// Serve the Session resource until the client releases it.
	sessResource := s.newSessionResource(sess)
	id, err := resourceCtx.AddResource(sessResource.GetMux(), func() {
		sessResource.Close()
		sessRef.Release()
	})
	if err != nil {
		sessResource.Close()
		sessRef.Release()
		return nil, err
	}
	return &s4wave_root.MountSessionResponse{ResourceId: id}, nil
}

// MountSessionByIdx mounts a session by index and returns the Session resource.
func (s *CoreRootServer) MountSessionByIdx(
	ctx context.Context,
	req *s4wave_root.MountSessionByIdxRequest,
) (*s4wave_root.MountSessionByIdxResponse, error) {
	// Find the caller's resource client.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Resolve the Session at the index.
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, s.b, "", false, nil)
	if err != nil {
		return nil, err
	}
	defer sessionCtrlRef.Release()
	sessInfo, err := sessionCtrl.GetSessionByIdx(ctx, req.GetSessionIdx())
	if err != nil {
		return nil, err
	}
	if sessInfo == nil {
		return &s4wave_root.MountSessionByIdxResponse{NotFound: true}, nil
	}

	// Mount the Session.
	sess, sessRef, err := session.ExMountSession(ctx, s.b, sessInfo.GetSessionRef(), false, nil)
	if err != nil {
		return nil, err
	}

	// Serve the Session resource until the client releases it.
	sessResource := s.newSessionResource(sess)
	id, err := resourceCtx.AddResource(sessResource.GetMux(), func() {
		sessResource.Close()
		sessRef.Release()
	})
	if err != nil {
		sessResource.Close()
		sessRef.Release()
		return nil, err
	}
	return &s4wave_root.MountSessionByIdxResponse{
		ResourceId: id,
		SessionRef: sessInfo.GetSessionRef(),
	}, nil
}

// newSessionResource builds the SessionResource this root serves for sess.
// Call Close when done.
func (s *CoreRootServer) newSessionResource(sess session.Session) *resource_session.SessionResource {
	// Configure the resource with this root's plugins, bindings, and CDN hooks.
	sessResource := resource_session.NewSessionResourceWithHostPluginIDAndRecoveryStatus(
		sess.GetSessionRef().GetLogger(s.le),
		s.b,
		sess,
		s.hostPluginID,
		s.recoveryStatusRegistry,
	)
	sessResource.SetAppPluginIDs(s.appPluginIDs)
	sessResource.SetBindingRegistry(s.bindingRegistry)
	sessResource.SetCdnRootChangedHook(func(spaceID string) {
		s.cdnRegistry.NotifyRootChanged(spaceID)
	})
	sessResource.SetCdnLookup(s.lookupCdnSharedObject)
	return sessResource
}

// ListSessions lists the configured sessions.
func (s *CoreRootServer) ListSessions(
	ctx context.Context,
	req *s4wave_root.ListSessionsRequest,
) (*s4wave_root.ListSessionsResponse, error) {
	// Acquire the Session controller for the configured session list.
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, s.b, "", false, nil)
	if err != nil {
		return nil, err
	}
	defer sessionCtrlRef.Release()

	// Read the configured Session entries.
	entries, err := sessionCtrl.ListSessions(ctx)
	if err != nil {
		return nil, err
	}

	return &s4wave_root.ListSessionsResponse{Sessions: entries}, nil
}

// GetSessionMetadata returns metadata for a session by index.
func (s *CoreRootServer) GetSessionMetadata(
	ctx context.Context,
	req *s4wave_root.GetSessionMetadataRequest,
) (*s4wave_root.GetSessionMetadataResponse, error) {
	// Acquire the Session controller for metadata lookup.
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, s.b, "", false, nil)
	if err != nil {
		return nil, err
	}
	defer sessionCtrlRef.Release()

	// Read metadata for the requested Session index.
	meta, err := s.sessionMetadata(ctx, sessionCtrl, req.GetSessionIdx())
	if err != nil {
		return nil, err
	}

	return &s4wave_root.GetSessionMetadataResponse{Metadata: meta}, nil
}

// WatchSessionMetadata streams metadata for a session by index.
func (s *CoreRootServer) WatchSessionMetadata(
	req *s4wave_root.WatchSessionMetadataRequest,
	strm s4wave_root.SRPCRootResourceService_WatchSessionMetadataStream,
) error {
	// Acquire the Session controller for the metadata stream.
	ctx := strm.Context()
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, s.b, "", false, nil)
	if err != nil {
		return err
	}
	defer sessionCtrlRef.Release()

	// Watch Session mutations and send changed metadata snapshots.
	bcast := sessionCtrl.GetSessionBroadcast()
	var prev *s4wave_root.WatchSessionMetadataResponse
	for {
		// Obtain the wait channel first so any mutation that lands after
		// this point will wake us. Then read the current state - this
		// ordering ensures no missed wakeups.
		var ch <-chan struct{}
		bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
		})

		meta, err := s.sessionMetadata(ctx, sessionCtrl, req.GetSessionIdx())
		if err != nil {
			return err
		}
		resp := &s4wave_root.WatchSessionMetadataResponse{
			Metadata: meta,
			NotFound: meta == nil,
		}
		if prev == nil || !resp.EqualVT(prev) {
			if err := strm.Send(resp); err != nil {
				return err
			}
			prev = resp
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}

func (s *CoreRootServer) sessionMetadata(ctx context.Context, sessionCtrl session.SessionController, sessionIdx uint32) (*session.SessionMetadata, error) {
	// Read the Session metadata before enriching PIN recovery state.
	meta, err := sessionCtrl.GetSessionMetadata(ctx, sessionIdx)
	if err != nil || meta == nil {
		return meta, err
	}
	meta = meta.CloneVT()
	if meta.GetLockMode() != session.SessionLockMode_SESSION_LOCK_MODE_PIN_ENCRYPTED {
		return meta, nil
	}

	// Resolve the Session provider account for PIN recovery lookup.
	sessInfo, err := sessionCtrl.GetSessionByIdx(ctx, sessionIdx)
	if err != nil || sessInfo == nil {
		return meta, err
	}
	ref := sessInfo.GetSessionRef()
	provRef := ref.GetProviderResourceRef()
	provAcc, provAccRef, err := provider.ExAccessProviderAccount(
		ctx, s.b,
		provRef.GetProviderId(),
		provRef.GetProviderAccountId(),
		false, nil,
	)
	if err != nil {
		return meta, nil
	}
	defer provAccRef.Release()

	// Read the provider account feature and its PIN recovery state.
	sessFeature, err := session.GetSessionProviderAccountFeature(ctx, provAcc)
	if err != nil {
		return meta, nil
	}
	recoveryState, err := sessFeature.GetPINSessionRecoveryState(ctx, ref)
	if err != nil {
		return meta, nil
	}
	meta.RecoveryState = recoveryState
	return meta, nil
}

// UnlockSession unlocks a PIN-locked session before mounting.
func (s *CoreRootServer) UnlockSession(
	ctx context.Context,
	req *s4wave_root.UnlockSessionByIdxRequest,
) (*s4wave_root.UnlockSessionByIdxResponse, error) {
	// Acquire the Session controller for the unlock request.
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, s.b, "", false, nil)
	if err != nil {
		return nil, err
	}
	defer sessionCtrlRef.Release()

	// Find the Session identified by the requested index.
	sessInfo, err := sessionCtrl.GetSessionByIdx(ctx, req.GetSessionIdx())
	if err != nil {
		return nil, err
	}
	if sessInfo == nil {
		return nil, session.ErrSessionNotFound
	}

	// Resolve the provider account reference for the locked Session.
	ref := sessInfo.GetSessionRef()
	provRef := ref.GetProviderResourceRef()

	// Access the provider account for the unlock request.
	provAcc, provAccRef, err := provider.ExAccessProviderAccount(
		ctx, s.b,
		provRef.GetProviderId(),
		provRef.GetProviderAccountId(),
		false, nil,
	)
	if err != nil {
		return nil, err
	}
	defer provAccRef.Release()

	// Resolve the provider account feature that unlocks PIN sessions.
	sessFeature, err := session.GetSessionProviderAccountFeature(ctx, provAcc)
	if err != nil {
		return nil, err
	}

	// Unlock the Session with the supplied PIN.
	if err := sessFeature.UnlockPINSession(ctx, ref, req.GetPin()); err != nil {
		return nil, err
	}

	return &s4wave_root.UnlockSessionByIdxResponse{}, nil
}

// WatchSessions streams the session list, sending updates when sessions change.
func (s *CoreRootServer) WatchSessions(
	req *s4wave_root.WatchSessionsRequest,
	strm s4wave_root.SRPCRootResourceService_WatchSessionsStream,
) error {
	// Acquire the Session controller for the session list stream.
	ctx := strm.Context()
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, s.b, "", false, nil)
	if err != nil {
		return err
	}
	defer sessionCtrlRef.Release()

	// Watch Session mutations and send changed session lists.
	bcast := sessionCtrl.GetSessionBroadcast()
	var prev *s4wave_root.WatchSessionsResponse
	for {
		var ch <-chan struct{}
		bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
		})

		entries, err := sessionCtrl.ListSessions(ctx)
		if err != nil {
			return err
		}
		resp := &s4wave_root.WatchSessionsResponse{Sessions: entries}
		if prev == nil || !resp.EqualVT(prev) {
			if err := strm.Send(resp); err != nil {
				return err
			}
			prev = resp
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}

type accountStatusWatcher interface {
	GetAccountBroadcast() *broadcast.Broadcast
	GetAccountStatus() provider.ProviderAccountStatus
}

// WatchAllAccountStatuses streams provider account statuses for all sessions.
func (s *CoreRootServer) WatchAllAccountStatuses(
	req *s4wave_root.WatchAllAccountStatusesRequest,
	strm s4wave_root.SRPCRootResourceService_WatchAllAccountStatusesStream,
) error {
	// Acquire the Session controller for the account status stream.
	ctx := strm.Context()
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, s.b, "", false, nil)
	if err != nil {
		return err
	}
	defer sessionCtrlRef.Release()

	// Send changed account statuses and wait on their retained watches.
	var prev *s4wave_root.WatchAllAccountStatusesResponse
	for {
		resp, waitChs, releases, err := s.snapshotAllAccountStatuses(ctx, sessionCtrl)
		if err != nil {
			for _, release := range releases {
				release()
			}
			return err
		}
		if prev == nil || !resp.EqualVT(prev) {
			if err := strm.Send(resp); err != nil {
				for _, release := range releases {
					release()
				}
				return err
			}
			prev = resp
		}

		ctxDone := waitAny(ctx, waitChs)
		for _, release := range releases {
			release()
		}
		if ctxDone {
			return ctx.Err()
		}
	}
}

func waitAny(ctx context.Context, waitChs []<-chan struct{}) bool {
	return broadcast.WaitAny(ctx, waitChs...) != nil
}

func (s *CoreRootServer) snapshotAllAccountStatuses(
	ctx context.Context,
	sessionCtrl session.SessionController,
) (
	*s4wave_root.WatchAllAccountStatusesResponse,
	[]<-chan struct{},
	[]func(),
	error,
) {
	// Capture the Session change notification before reading entries.
	var sessionCh <-chan struct{}
	sessionCtrl.GetSessionBroadcast().HoldLock(func(
		_ func(),
		getWaitCh func() <-chan struct{},
	) {
		sessionCh = getWaitCh()
	})

	// Read the Session entries whose account statuses will be reported.
	entries, err := sessionCtrl.ListSessions(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

	// Collect each Session status and retain its provider account watch.
	accountStatuses := make(map[string]provider.ProviderAccountStatus)
	accountWaitChs := make(map[string]<-chan struct{})
	releases := make([]func(), 0, len(entries))
	rows := make([]*s4wave_root.SessionAccountStatus, 0, len(entries))
	for _, entry := range entries {
		status := provider.ProviderAccountStatus_ProviderAccountStatus_READY
		provRef := entry.GetSessionRef().GetProviderResourceRef()
		if provRef == nil {
			status = provider.ProviderAccountStatus_ProviderAccountStatus_NONE
		} else if provRef.GetProviderId() == "spacewave" {
			accountID := provRef.GetProviderAccountId()
			if cached, ok := accountStatuses[accountID]; ok {
				status = cached
			} else {
				acc, accRef, err := provider.ExAccessProviderAccount(
					ctx,
					s.b,
					provRef.GetProviderId(),
					accountID,
					false,
					nil,
				)
				if err != nil {
					status = provider.ProviderAccountStatus_ProviderAccountStatus_NONE
				} else {
					releases = append(releases, accRef.Release)
					watcher, ok := acc.(accountStatusWatcher)
					if !ok {
						status = provider.ProviderAccountStatus_ProviderAccountStatus_NONE
					} else {
						watcher.GetAccountBroadcast().HoldLock(func(
							_ func(),
							getWaitCh func() <-chan struct{},
						) {
							status = watcher.GetAccountStatus()
							accountWaitChs[accountID] = getWaitCh()
						})
					}
				}
				accountStatuses[accountID] = status
			}
		}

		rows = append(rows, &s4wave_root.SessionAccountStatus{
			SessionIdx:    entry.GetSessionIndex(),
			AccountStatus: status,
		})
	}
	slices.SortFunc(rows, func(a, b *s4wave_root.SessionAccountStatus) int {
		switch {
		case a.GetSessionIdx() < b.GetSessionIdx():
			return -1
		case a.GetSessionIdx() > b.GetSessionIdx():
			return 1
		default:
			return 0
		}
	})

	// Combine Session and provider account change notifications.
	waitChs := make([]<-chan struct{}, 0, len(accountWaitChs)+1)
	waitChs = append(waitChs, sessionCh)
	for _, ch := range accountWaitChs {
		waitChs = append(waitChs, ch)
	}

	return &s4wave_root.WatchAllAccountStatusesResponse{Statuses: rows}, waitChs, releases, nil
}

// DeleteSession removes a session from the local session list by index.
func (s *CoreRootServer) DeleteSession(
	ctx context.Context,
	req *s4wave_root.DeleteSessionRequest,
) (*s4wave_root.DeleteSessionResponse, error) {
	// Acquire the Session controller for the delete request.
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, s.b, "", false, nil)
	if err != nil {
		return nil, err
	}
	defer sessionCtrlRef.Release()

	// Find the Session to remove from the local list.
	sessInfo, err := sessionCtrl.GetSessionByIdx(ctx, req.GetSessionIdx())
	if err != nil {
		return nil, err
	}
	if sessInfo == nil {
		return &s4wave_root.DeleteSessionResponse{}, nil
	}

	// Delete the selected Session from the controller.
	if err := sessionCtrl.DeleteSession(ctx, sessInfo.GetSessionRef()); err != nil {
		return nil, err
	}

	return &s4wave_root.DeleteSessionResponse{}, nil
}

// ResetSession resets a PIN-locked session via entity key verification.
func (s *CoreRootServer) ResetSession(
	ctx context.Context,
	req *s4wave_root.ResetSessionByIdxRequest,
) (*s4wave_root.ResetSessionByIdxResponse, error) {
	// Require the credential used to verify the Session reset.
	cred := req.GetCredential()
	if cred == nil {
		return nil, errors.New("credential is required")
	}

	// Acquire the Session controller for the reset request.
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, s.b, "", false, nil)
	if err != nil {
		return nil, err
	}
	defer sessionCtrlRef.Release()

	// Find the Session identified by the reset request.
	sessInfo, err := sessionCtrl.GetSessionByIdx(ctx, req.GetSessionIdx())
	if err != nil {
		return nil, err
	}
	if sessInfo == nil {
		return nil, session.ErrSessionNotFound
	}

	// Resolve the provider account reference for the Session reset.
	ref := sessInfo.GetSessionRef()
	provRef := ref.GetProviderResourceRef()

	// Access the provider account for credential verification and reset.
	provAcc, provAccRef, err := provider.ExAccessProviderAccount(
		ctx, s.b,
		provRef.GetProviderId(),
		provRef.GetProviderAccountId(),
		false, nil,
	)
	if err != nil {
		return nil, err
	}
	defer provAccRef.Release()

	// Verify the credential through the Spacewave account resource.
	if provRef.GetProviderId() == "spacewave" {
		accResource := resource_account.NewAccountResource(provAcc)
		if accResource == nil {
			return nil, errors.New("spacewave account resource unavailable")
		}
		defer accResource.Release()
		if _, _, err := accResource.ResolveEntityKey(ctx, cred); err != nil {
			return nil, errors.Wrap(err, "verify credential")
		}
	}

	// Resolve the provider account feature that resets PIN sessions.
	sessFeature, err := session.GetSessionProviderAccountFeature(ctx, provAcc)
	if err != nil {
		return nil, err
	}

	// Reset the Session using the verified credential.
	if err := sessFeature.ResetPINSession(ctx, ref, cred); err != nil {
		return nil, err
	}

	return &s4wave_root.ResetSessionByIdxResponse{}, nil
}
