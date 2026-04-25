package resource_session

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	provider_transfer "github.com/s4wave/spacewave/core/provider/transfer"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/volume"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	"github.com/sirupsen/logrus"
)

// transferManager tracks an active transfer operation on a session resource.
type transferManager struct {
	mtx      sync.Mutex
	transfer *provider_transfer.Transfer
	rc       *routine.RoutineContainer
	running  bool
}

// GetActiveTransfer returns the currently active transfer, or nil if none.
func (r *SessionResource) GetActiveTransfer() *provider_transfer.Transfer {
	r.transferMgr.mtx.Lock()
	defer r.transferMgr.mtx.Unlock()
	return r.transferMgr.transfer
}

// GetTransferInventory returns the list of spaces on a session for transfer planning.
func (r *SessionResource) GetTransferInventory(
	ctx context.Context,
	req *s4wave_session.GetTransferInventoryRequest,
) (*s4wave_session.GetTransferInventoryResponse, error) {
	sessionIdx := req.GetSessionIndex()
	if sessionIdx == 0 {
		return nil, errors.New("session_index is required")
	}

	// Look up the target session by index.
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, r.b, "", false, nil)
	if err != nil {
		return nil, errors.Wrap(err, "lookup session controller")
	}
	defer sessionCtrlRef.Release()

	sessInfo, err := sessionCtrl.GetSessionByIdx(ctx, sessionIdx)
	if err != nil {
		return nil, errors.Wrap(err, "get session by index")
	}
	if sessInfo == nil {
		return nil, session.ErrSessionNotFound
	}

	// Access the provider account for that session.
	provRef := sessInfo.GetSessionRef().GetProviderResourceRef()
	provAcc, provAccRef, err := provider.ExAccessProviderAccount(
		ctx, r.b,
		provRef.GetProviderId(),
		provRef.GetProviderAccountId(),
		false, nil,
	)
	if err != nil {
		return nil, errors.Wrap(err, "access provider account")
	}
	defer provAccRef.Release()

	// Get the shared object list.
	soFeature, err := sobject.GetSharedObjectProviderAccountFeature(ctx, provAcc)
	if err != nil {
		return nil, errors.Wrap(err, "get shared object feature")
	}

	soListWatchable, relSoList, err := soFeature.AccessSharedObjectList(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "access shared object list")
	}
	defer relSoList()

	soList, err := soListWatchable.WaitValue(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "wait for shared object list")
	}

	// Filter to spaces.
	spaces, err := space.FilterSharedObjectList(soList.GetSharedObjects(), func(_ *sobject.SharedObjectListEntry, _ error) error {
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "filter shared object list")
	}

	// The well-known CDN Space is not a transferable resource.
	spaces = filterOutCdnSpace(spaces)

	return &s4wave_session.GetTransferInventoryResponse{Spaces: spaces}, nil
}

// StartTransfer starts a transfer operation between two sessions.
func (r *SessionResource) StartTransfer(
	ctx context.Context,
	req *s4wave_session.StartTransferRequest,
) (*s4wave_session.StartTransferResponse, error) {
	mode := req.GetMode()
	if mode == provider_transfer.TransferMode_TransferMode_UNKNOWN {
		return nil, errors.New("transfer mode is required")
	}
	srcIdx := req.GetSourceSessionIndex()
	tgtIdx := req.GetTargetSessionIndex()
	if srcIdx == 0 || tgtIdx == 0 {
		return nil, errors.New("source and target session indexes are required")
	}
	if srcIdx == tgtIdx {
		return nil, errors.New("source and target sessions must be different")
	}

	// Check for existing active transfer.
	r.transferMgr.mtx.Lock()
	if r.transferMgr.running {
		r.transferMgr.mtx.Unlock()
		return nil, errors.New("a transfer is already in progress")
	}
	r.transferMgr.mtx.Unlock()

	// Look up session controller.
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, r.b, "", false, nil)
	if err != nil {
		return nil, errors.Wrap(err, "lookup session controller")
	}
	defer sessionCtrlRef.Release()

	// Look up source and target sessions.
	srcEntry, err := sessionCtrl.GetSessionByIdx(ctx, srcIdx)
	if err != nil {
		return nil, errors.Wrap(err, "get source session")
	}
	if srcEntry == nil {
		return nil, errors.New("source session not found")
	}
	tgtEntry, err := sessionCtrl.GetSessionByIdx(ctx, tgtIdx)
	if err != nil {
		return nil, errors.Wrap(err, "get target session")
	}
	if tgtEntry == nil {
		return nil, errors.New("target session not found")
	}

	// Build source and target transfer adapters.
	source, cleanup, err := buildTransferSource(ctx, r.b, srcEntry)
	if err != nil {
		return nil, errors.Wrap(err, "build transfer source")
	}
	target, err := buildTransferTarget(ctx, r.b, tgtEntry)
	if err != nil {
		return nil, errors.Wrap(err, "build transfer target")
	}

	// Build checkpoint store on source account.
	checkpoint, err := buildCheckpointStore(ctx, r.b, srcEntry)
	if err != nil {
		return nil, errors.Wrap(err, "build checkpoint store")
	}

	le := r.le.WithFields(logrus.Fields{
		"transfer-src":  srcIdx,
		"transfer-tgt":  tgtIdx,
		"transfer-mode": mode.String(),
	})

	// Build a state rewriter to re-key SO state from source to target peer.
	stateRewriter, err := buildStateRewriter(ctx, le, source, target)
	if err != nil {
		return nil, errors.Wrap(err, "build state rewriter")
	}

	xfer := provider_transfer.NewTransfer(
		le, mode, source, target, srcIdx, tgtIdx, cleanup, checkpoint, stateRewriter, req.GetSpaceIds(),
	)

	// Read the linked-cloud account ID before the transfer starts, because the
	// cleanup phase will delete the source volume (making the ObjectStore
	// inaccessible).
	var srcLinkedCloudAccountID string
	if mode == provider_transfer.TransferMode_TransferMode_MERGE || mode == provider_transfer.TransferMode_TransferMode_MIGRATE {
		srcLinkedCloudAccountID, err = readLinkedCloudAccountID(ctx, r.b, srcEntry, source)
		if err != nil {
			return nil, errors.Wrap(err, "read linked-cloud account id")
		}
	}

	var rc *routine.RoutineContainer
	rc = routine.NewRoutineContainerWithLogger(
		le.WithField("routine", "transfer"),
		routine.WithExitCb(func(_ error) {
			r.transferMgr.mtx.Lock()
			if r.transferMgr.rc == rc {
				r.transferMgr.running = false
			}
			r.transferMgr.mtx.Unlock()
		}),
	)
	rc.SetRoutine(func(ctx context.Context) error {
		err := xfer.Execute(ctx)
		if err != nil {
			return err
		}

		// For MIGRATE mode, copy the source keypair to the target so it retains
		// the same peer identity.
		if mode == provider_transfer.TransferMode_TransferMode_MIGRATE {
			srcVol := getTransferSourceVolume(source)
			tgtVol := getTransferTargetVolume(target)
			if srcVol != nil && tgtVol != nil {
				if kerr := provider_transfer.TransferKeypair(ctx, srcVol, tgtVol); kerr != nil {
					return xfer.Fail(errors.Wrap(kerr, "transfer keypair"))
				}
			}
		}

		// After merge or migrate, clean up linked-cloud ref and delete the source session.
		if mode == provider_transfer.TransferMode_TransferMode_MERGE || mode == provider_transfer.TransferMode_TransferMode_MIGRATE {
			sessCtrl, sessCtrlRef, lerr := session.ExLookupSessionController(ctx, r.b, "", false, nil)
			if lerr != nil {
				return xfer.Fail(errors.Wrap(lerr, "lookup session controller for cleanup"))
			}
			defer sessCtrlRef.Release()
			if srcLinkedCloudAccountID != "" {
				if lerr := cleanupLinkedCloudRef(ctx, le, r.b, sessCtrl, srcLinkedCloudAccountID); lerr != nil {
					return xfer.Fail(errors.Wrap(lerr, "cleanup linked-cloud ref"))
				}
			}
			if lerr := sessCtrl.DeleteSession(ctx, srcEntry.GetSessionRef()); lerr != nil {
				return xfer.Fail(errors.Wrap(lerr, "delete source session"))
			}
		}
		return nil
	})

	r.transferMgr.mtx.Lock()
	r.transferMgr.transfer = xfer
	r.transferMgr.rc = rc
	r.transferMgr.running = true
	r.transferMgr.mtx.Unlock()
	rc.SetContext(r.ctx, false)
	for {
		state := xfer.GetState()
		if state.GetPhase() != provider_transfer.TransferPhase_TransferPhase_IDLE {
			break
		}
		select {
		case <-ctx.Done():
			rc.ClearContext()
			return nil, ctx.Err()
		case <-xfer.WaitState():
		}
	}

	return &s4wave_session.StartTransferResponse{}, nil
}

// WatchTransferProgress streams transfer state updates for an active transfer.
func (r *SessionResource) WatchTransferProgress(
	req *s4wave_session.WatchTransferProgressRequest,
	strm s4wave_session.SRPCSessionResourceService_WatchTransferProgressStream,
) error {
	ctx := strm.Context()

	r.transferMgr.mtx.Lock()
	xfer := r.transferMgr.transfer
	r.transferMgr.mtx.Unlock()

	if xfer == nil {
		return errors.New("no transfer in progress")
	}

	var prev *provider_transfer.TransferState
	for {
		ch := xfer.WaitState()
		state := xfer.GetState()

		if prev == nil || !state.EqualVT(prev) {
			if err := strm.Send(&s4wave_session.WatchTransferProgressResponse{
				State: state,
			}); err != nil {
				return err
			}
			prev = state
		}

		// If transfer is complete or failed, send final state and return.
		phase := state.GetPhase()
		if phase == provider_transfer.TransferPhase_TransferPhase_COMPLETE ||
			phase == provider_transfer.TransferPhase_TransferPhase_FAILED {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}

// CancelTransfer stops an in-progress transfer.
func (r *SessionResource) CancelTransfer(
	ctx context.Context,
	req *s4wave_session.CancelTransferRequest,
) (*s4wave_session.CancelTransferResponse, error) {
	r.transferMgr.mtx.Lock()
	rc := r.transferMgr.rc
	running := r.transferMgr.running
	r.transferMgr.mtx.Unlock()

	if rc == nil || !running {
		return nil, errors.New("no transfer in progress")
	}

	waitCh, _ := rc.SetRoutine(nil)
	if waitCh != nil {
		select {
		case <-waitCh:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	r.transferMgr.mtx.Lock()
	if r.transferMgr.rc == rc {
		r.transferMgr.running = false
	}
	r.transferMgr.mtx.Unlock()

	return &s4wave_session.CancelTransferResponse{}, nil
}

// GetTransferStatus returns whether a transfer is active or a checkpoint exists.
func (r *SessionResource) GetTransferStatus(
	ctx context.Context,
	req *s4wave_session.GetTransferStatusRequest,
) (*s4wave_session.GetTransferStatusResponse, error) {
	// Check for active in-memory transfer.
	r.transferMgr.mtx.Lock()
	xfer := r.transferMgr.transfer
	r.transferMgr.mtx.Unlock()

	if xfer != nil {
		state := xfer.GetState()
		phase := state.GetPhase()
		if phase != provider_transfer.TransferPhase_TransferPhase_COMPLETE &&
			phase != provider_transfer.TransferPhase_TransferPhase_FAILED {
			return &s4wave_session.GetTransferStatusResponse{
				Active: true,
				State:  state,
			}, nil
		}
	}

	// Check for checkpoint from interrupted transfer on the current session.
	// Only local providers support checkpoints.
	localAcc, ok := r.session.GetProviderAccount().(*provider_local.ProviderAccount)
	if !ok {
		return &s4wave_session.GetTransferStatusResponse{}, nil
	}

	provRef := r.session.GetSessionRef().GetProviderResourceRef()
	objStoreID := provider_local.SobjectObjectStoreID(provRef.GetProviderId(), provRef.GetProviderAccountId())
	volID := localAcc.GetVolume().GetID()
	cpStore := provider_transfer.NewObjectStoreCheckpointLazy(r.b, objStoreID, volID)

	cp, err := cpStore.LoadCheckpoint(ctx)
	if err != nil || cp == nil {
		return &s4wave_session.GetTransferStatusResponse{}, nil
	}

	return &s4wave_session.GetTransferStatusResponse{
		HasCheckpoint: true,
		State:         cp.GetState(),
	}, nil
}

// buildTransferSource creates a TransferSource and optional CleanupSource from a session entry.
func buildTransferSource(
	ctx context.Context,
	b bus.Bus,
	entry *session.SessionListEntry,
) (provider_transfer.TransferSource, provider_transfer.CleanupSource, error) {
	provRef := entry.GetSessionRef().GetProviderResourceRef()
	provID := provRef.GetProviderId()
	accountID := provRef.GetProviderAccountId()

	provAcc, provAccRef, err := provider.ExAccessProviderAccount(ctx, b, provID, accountID, false, nil)
	if err != nil {
		return nil, nil, err
	}
	defer provAccRef.Release()

	switch acc := provAcc.(type) {
	case *provider_local.ProviderAccount:
		src := provider_transfer.NewLocalTransferSource(acc, provID, accountID, b)
		return src, src, nil
	case *provider_spacewave.ProviderAccount:
		src := provider_transfer.NewSpacewaveTransferSource(acc, provID, accountID)
		return src, nil, nil
	default:
		return nil, nil, errors.New("unsupported provider type for transfer source")
	}
}

// buildTransferTarget creates a TransferTarget from a session entry.
func buildTransferTarget(
	ctx context.Context,
	b bus.Bus,
	entry *session.SessionListEntry,
) (provider_transfer.TransferTarget, error) {
	provRef := entry.GetSessionRef().GetProviderResourceRef()
	provID := provRef.GetProviderId()
	accountID := provRef.GetProviderAccountId()

	provAcc, provAccRef, err := provider.ExAccessProviderAccount(ctx, b, provID, accountID, false, nil)
	if err != nil {
		return nil, err
	}
	defer provAccRef.Release()

	switch acc := provAcc.(type) {
	case *provider_local.ProviderAccount:
		return provider_transfer.NewLocalTransferTarget(acc, provID, accountID, b), nil
	case *provider_spacewave.ProviderAccount:
		// Cloud targets require a write-eligible subscription for block store writes.
		subStatus, err := acc.GetSubscriptionStatus(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "check target subscription status")
		}
		if !s4wave_provider_spacewave.BillingStatusFromString(subStatus).IsWriteAllowed() {
			return nil, errors.Errorf("target cloud account subscription is %q: an active subscription is required to transfer data", subStatus)
		}
		return provider_transfer.NewSpacewaveTransferTarget(acc, provID, accountID), nil
	default:
		return nil, errors.New("unsupported provider type for transfer target")
	}
}

// buildCheckpointStore creates a CheckpointStore from a session entry.
func buildCheckpointStore(
	ctx context.Context,
	b bus.Bus,
	entry *session.SessionListEntry,
) (provider_transfer.CheckpointStore, error) {
	provRef := entry.GetSessionRef().GetProviderResourceRef()
	provID := provRef.GetProviderId()
	accountID := provRef.GetProviderAccountId()

	provAcc, provAccRef, err := provider.ExAccessProviderAccount(ctx, b, provID, accountID, false, nil)
	if err != nil {
		return nil, err
	}
	defer provAccRef.Release()

	switch acc := provAcc.(type) {
	case *provider_local.ProviderAccount:
		objStoreID := provider_local.SobjectObjectStoreID(provID, accountID)
		volID := acc.GetVolume().GetID()
		return provider_transfer.NewObjectStoreCheckpointLazy(b, objStoreID, volID), nil
	case *provider_spacewave.ProviderAccount:
		return nil, nil
	default:
		return nil, errors.New("unsupported provider type for checkpoint")
	}
}

// buildStateRewriter builds an SOStateRewriter that re-keys SO state from
// source peer to target peer. Returns nil if neither side is a local account
// (no re-keying needed when keys are the same).
func buildStateRewriter(
	ctx context.Context,
	le *logrus.Entry,
	source provider_transfer.TransferSource,
	target provider_transfer.TransferTarget,
) (provider_transfer.SOStateRewriter, error) {
	srcVol := getTransferSourceVolume(source)
	tgtVol := getTransferTargetVolume(target)
	if srcVol == nil || tgtVol == nil {
		return nil, nil
	}

	sfs := getTransferSourceStepFactorySet(source)
	if sfs == nil {
		return nil, nil
	}

	srcPeer, err := srcVol.GetPeer(ctx, true)
	if err != nil {
		return nil, errors.Wrap(err, "get source peer")
	}
	tgtPeer, err := tgtVol.GetPeer(ctx, true)
	if err != nil {
		return nil, errors.Wrap(err, "get target peer")
	}
	srcPriv, err := srcPeer.GetPrivKey(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get source private key")
	}
	tgtPriv, err := tgtPeer.GetPrivKey(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get target private key")
	}

	if srcPeer.GetPeerID().String() == tgtPeer.GetPeerID().String() {
		return nil, nil
	}

	return func(ctx context.Context, soID string, state *sobject.SOState) (*sobject.SOState, error) {
		return provider_transfer.RekeySOState(ctx, le, sfs, state, srcPriv, tgtPriv, soID)
	}, nil
}

// getTransferSourceVolume extracts the volume from a transfer source.
func getTransferSourceVolume(source provider_transfer.TransferSource) volume.Volume {
	switch s := source.(type) {
	case *provider_transfer.LocalTransferSource:
		return s.GetAccount().GetVolume()
	case *provider_transfer.SpacewaveTransferSource:
		return s.GetAccount().GetVolume()
	}
	return nil
}

// getTransferTargetVolume extracts the volume from a transfer target.
func getTransferTargetVolume(target provider_transfer.TransferTarget) volume.Volume {
	switch t := target.(type) {
	case *provider_transfer.LocalTransferTarget:
		return t.GetAccount().GetVolume()
	case *provider_transfer.SpacewaveTransferTarget:
		return t.GetAccount().GetVolume()
	}
	return nil
}

// getTransferSourceStepFactorySet extracts the block transform step factory set
// needed to decrypt source SO roots before re-keying.
func getTransferSourceStepFactorySet(source provider_transfer.TransferSource) *block_transform.StepFactorySet {
	switch s := source.(type) {
	case *provider_transfer.LocalTransferSource:
		return s.GetAccount().GetStepFactorySet()
	case *provider_transfer.SpacewaveTransferSource:
		return s.GetAccount().GetStepFactorySet()
	}
	return nil
}

// readLinkedCloudAccountID reads the linked-cloud account ID from a local
// session's ObjectStore. Returns empty string if the session is not local or
// has no cloud link. Uses the transfer source's volume ID to avoid the raw
// StorageVolumeID which may not match the proxy volume on the plugin bus.
func readLinkedCloudAccountID(ctx context.Context, b bus.Bus, entry *session.SessionListEntry, source provider_transfer.TransferSource) (string, error) {
	provRef := entry.GetSessionRef().GetProviderResourceRef()
	provID := provRef.GetProviderId()
	if provID != provider_local.ProviderID {
		return "", nil
	}

	localSrc, ok := source.(*provider_transfer.LocalTransferSource)
	if !ok {
		return "", nil
	}

	accountID := provRef.GetProviderAccountId()
	sessionID := provRef.GetId()
	objectStoreID := provider_local.SessionObjectStoreID(provID, accountID)
	volID := localSrc.GetAccount().GetVolume().GetID()

	objStoreHandle, _, diRef, err := volume.ExBuildObjectStoreAPI(ctx, b, false, objectStoreID, volID, nil)
	if err != nil {
		return "", errors.Wrap(err, "mount session object store")
	}
	defer diRef.Release()

	otx, err := objStoreHandle.GetObjectStore().NewTransaction(ctx, false)
	if err != nil {
		return "", errors.Wrap(err, "new read transaction")
	}
	defer otx.Discard()

	key := provider_local.LinkedCloudKey(sessionID)
	data, found, err := otx.Get(ctx, key)
	if err != nil {
		return "", errors.Wrap(err, "read linked-cloud key")
	}
	if !found {
		return "", nil
	}
	return string(data), nil
}

// cleanupLinkedCloudRef removes the cloud-side linked-local reference for a
// source session being merged or migrated away. The cloudAccountID must be
// read before the transfer starts (since the source volume is deleted during
// the cleanup phase).
func cleanupLinkedCloudRef(ctx context.Context, le *logrus.Entry, b bus.Bus, sessCtrl session.SessionController, cloudAccountID string) error {
	le.WithField("cloud-account-id", cloudAccountID).Info("cleaning up linked-cloud reference")

	sessions, err := sessCtrl.ListSessions(ctx)
	if err != nil {
		return errors.Wrap(err, "list sessions")
	}

	for _, entry := range sessions {
		provRef := entry.GetSessionRef().GetProviderResourceRef()
		if provRef.GetProviderId() != "spacewave" {
			continue
		}
		if provRef.GetProviderAccountId() != cloudAccountID {
			continue
		}

		// Access the spacewave account and delete the linked-local reference.
		swAcc, swAccRef, aerr := provider.ExAccessProviderAccount(ctx, b, "spacewave", cloudAccountID, false, nil)
		if aerr != nil {
			return errors.Wrap(aerr, "access spacewave account")
		}
		defer swAccRef.Release()

		cloudSessionID := provRef.GetId()
		swAccTyped, ok := swAcc.(*provider_spacewave.ProviderAccount)
		if !ok {
			return errors.New("spacewave account is not the expected type")
		}
		if derr := swAccTyped.DeleteLinkedLocalSession(ctx, cloudSessionID); derr != nil {
			return errors.Wrap(derr, "delete linked-local reference")
		}
		le.Info("cleaned up linked-local reference on cloud side")
		return nil
	}

	return errors.New("no spacewave session found for linked cloud account")
}
