package resource_root

import (
	"context"
	"slices"
	"sync"

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
	if err := req.GetSessionRef().Validate(); err != nil {
		return nil, err
	}

	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	sess, sessRef, err := session.ExMountSession(ctx, s.b, req.GetSessionRef(), false, nil)
	if err != nil {
		return nil, err
	}

	le := sess.GetSessionRef().GetLogger(s.le)
	sessResource := resource_session.NewSessionResource(le, s.b, sess)
	sessResource.SetCdnRootChangedHook(func(spaceID string) {
		s.cdnRegistry.NotifyRootChanged(spaceID)
	})
	sessResource.SetCdnLookup(s.lookupCdnSharedObject)
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
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

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

	sess, sessRef, err := session.ExMountSession(ctx, s.b, sessInfo.GetSessionRef(), false, nil)
	if err != nil {
		return nil, err
	}

	le := sess.GetSessionRef().GetLogger(s.le)
	sessResource := resource_session.NewSessionResource(le, s.b, sess)
	sessResource.SetCdnRootChangedHook(func(spaceID string) {
		s.cdnRegistry.NotifyRootChanged(spaceID)
	})
	sessResource.SetCdnLookup(s.lookupCdnSharedObject)
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

// ListSessions lists the configured sessions.
func (s *CoreRootServer) ListSessions(
	ctx context.Context,
	req *s4wave_root.ListSessionsRequest,
) (*s4wave_root.ListSessionsResponse, error) {
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, s.b, "", false, nil)
	if err != nil {
		return nil, err
	}
	defer sessionCtrlRef.Release()

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
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, s.b, "", false, nil)
	if err != nil {
		return nil, err
	}
	defer sessionCtrlRef.Release()

	meta, err := sessionCtrl.GetSessionMetadata(ctx, req.GetSessionIdx())
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
	ctx := strm.Context()
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, s.b, "", false, nil)
	if err != nil {
		return err
	}
	defer sessionCtrlRef.Release()

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

		meta, err := sessionCtrl.GetSessionMetadata(ctx, req.GetSessionIdx())
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

// UnlockSession unlocks a PIN-locked session before mounting.
func (s *CoreRootServer) UnlockSession(
	ctx context.Context,
	req *s4wave_root.UnlockSessionByIdxRequest,
) (*s4wave_root.UnlockSessionByIdxResponse, error) {
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
		return nil, session.ErrSessionNotFound
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
		return nil, err
	}
	defer provAccRef.Release()

	sessFeature, err := session.GetSessionProviderAccountFeature(ctx, provAcc)
	if err != nil {
		return nil, err
	}

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
	ctx := strm.Context()
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, s.b, "", false, nil)
	if err != nil {
		return err
	}
	defer sessionCtrlRef.Release()

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
	ctx := strm.Context()
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, s.b, "", false, nil)
	if err != nil {
		return err
	}
	defer sessionCtrlRef.Release()

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
	switch len(waitChs) {
	case 0:
		<-ctx.Done()
		return true
	case 1:
		select {
		case <-ctx.Done():
			return true
		case <-waitChs[0]:
			return false
		}
	}

	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	done := make(chan bool, 1)
	var once sync.Once
	for _, ch := range waitChs {
		go func(ch <-chan struct{}) {
			select {
			case <-waitCtx.Done():
			case <-ch:
				once.Do(func() {
					done <- false
					cancel()
				})
			}
		}(ch)
	}

	select {
	case <-ctx.Done():
		once.Do(func() {
			done <- true
			cancel()
		})
		return true
	case ctxDone := <-done:
		return ctxDone
	}
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
	var sessionCh <-chan struct{}
	sessionCtrl.GetSessionBroadcast().HoldLock(func(
		_ func(),
		getWaitCh func() <-chan struct{},
	) {
		sessionCh = getWaitCh()
	})

	entries, err := sessionCtrl.ListSessions(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

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
		return &s4wave_root.DeleteSessionResponse{}, nil
	}

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
	cred := req.GetCredential()
	if cred == nil {
		return nil, errors.New("credential is required")
	}

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
		return nil, session.ErrSessionNotFound
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
		return nil, err
	}
	defer provAccRef.Release()

	// Verify credential for cloud sessions (AccountResource handles this).
	accResource := resource_account.NewAccountResource(provAcc)
	if accResource != nil {
		defer accResource.Release()
		if _, _, err := accResource.ResolveEntityKey(ctx, cred); err != nil {
			return nil, errors.Wrap(err, "verify credential")
		}
	}

	sessFeature, err := session.GetSessionProviderAccountFeature(ctx, provAcc)
	if err != nil {
		return nil, err
	}

	if err := sessFeature.ResetPINSession(ctx, ref, cred); err != nil {
		return nil, err
	}

	return &s4wave_root.ResetSessionByIdxResponse{}, nil
}
