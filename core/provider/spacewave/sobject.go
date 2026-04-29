package provider_spacewave

import (
	"context"
	"path"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/bstore"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_volume "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_org "github.com/s4wave/spacewave/sdk/org"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// SharedObject implements the sobject interface attached to sobjectTracker.
type SharedObject struct {
	tkr      *sobjectTracker
	blkStore bstore.BlockStore
	host     *cloudSOHost
	privKey  crypto.PrivKey
	localPid peer.ID
}

// GetBus returns the bus used for the shared object.
func (s *SharedObject) GetBus() bus.Bus {
	return s.tkr.a.p.b
}

// GetPeerID returns the local peer id for the shared object.
func (s *SharedObject) GetPeerID() peer.ID {
	return s.localPid
}

// GetSharedObjectID returns the shared object id.
func (s *SharedObject) GetSharedObjectID() string {
	return s.tkr.id
}

// GetBlockStore returns the block store mounted along with the SharedObject.
func (s *SharedObject) GetBlockStore() bstore.BlockStore {
	return s.blkStore
}

// AccessLocalStateStore accesses a kvtx ops for a local state store with the given ID.
func (s *SharedObject) AccessLocalStateStore(ctx context.Context, storeID string, released func()) (kvtx.Store, func(), error) {
	// For cloud provider, local state store is not yet implemented.
	return nil, nil, errors.New("local state store not available for cloud provider")
}

// GetSharedObjectState returns a snapshot of the shared object state.
func (s *SharedObject) GetSharedObjectState(ctx context.Context) (sobject.SharedObjectStateSnapshot, error) {
	stateCtr, relStateCtr, err := s.host.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer relStateCtr()

	soState, err := stateCtr.WaitValue(ctx, nil)
	if err != nil {
		return nil, err
	}

	snap := sobject.NewSOStateParticipantHandle(
		s.tkr.a.le,
		s.tkr.a.sfs,
		s.GetSharedObjectID(),
		soState,
		s.privKey,
		s.localPid,
	)
	return snap, nil
}

// AccessSharedObjectState adds a reference to the state and returns the state container.
func (s *SharedObject) AccessSharedObjectState(ctx context.Context, released func()) (ccontainer.Watchable[sobject.SharedObjectStateSnapshot], func(), error) {
	return s.host.AccessSharedObjectSnapshot(), func() {}, nil
}

// QueueOperation applies an operation to the shared object op queue.
func (s *SharedObject) QueueOperation(ctx context.Context, op []byte) (string, error) {
	snap, err := s.GetSharedObjectState(ctx)
	if err != nil {
		return "", err
	}

	xfrm, err := snap.GetTransformer(ctx)
	if err != nil {
		return "", err
	}

	encOp, err := xfrm.EncodeBlock(op)
	if err != nil {
		return "", err
	}

	id := sobject.NewSOOperationLocalID()
	err = s.host.QueueOperation(ctx, s.localPid, func(nonce uint64) (*sobject.SOOperation, error) {
		return sobject.BuildSOOperation(
			s.host.soHost.GetSharedObjectID(),
			s.privKey,
			encOp,
			nonce,
			id,
		)
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

// WaitOperation waits for the operation to be confirmed or rejected by the provider.
func (s *SharedObject) WaitOperation(ctx context.Context, localID string) (uint64, bool, error) {
	// Simple version: wait for state change.
	soStateCtr, relSoStateCtr, err := s.host.GetSOHost().GetSOStateCtr(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	defer relSoStateCtr()

	var current *sobject.SOState
	for {
		next, err := soStateCtr.WaitValueChange(ctx, current, nil)
		if err != nil {
			return 0, false, err
		}
		current = next

		// Check if the operation is still in the queue.
		found := false
		for _, op := range current.GetOps() {
			opInner, err := op.UnmarshalInner()
			if err != nil {
				continue
			}
			if opInner.GetLocalId() == localID {
				found = true
				break
			}
		}

		if found {
			// Still pending, wait for next change.
			continue
		}

		// Check for rejection.
		for _, peerRej := range current.GetOpRejections() {
			for _, rej := range peerRej.GetRejections() {
				rejInner := &sobject.SOOperationRejectionInner{}
				if err := rejInner.UnmarshalVT(rej.GetInner()); err != nil {
					continue
				}
				if rejInner.GetLocalId() == localID {
					return 0, true, sobject.ErrRejectedOp
				}
			}
		}

		// Not in queue and not rejected: accepted.
		return current.GetRoot().GetInnerSeqno(), false, nil
	}
}

// ClearOperationResult clears the operation state.
func (s *SharedObject) ClearOperationResult(ctx context.Context, localID string) error {
	// For cloud provider, no local op result store to clear.
	return nil
}

// ProcessOperations processes operations as a validator.
func (s *SharedObject) ProcessOperations(ctx context.Context, watch bool, cb sobject.ProcessOpsFunc) error {
	soStateCtr, relSoStateCtr, err := s.host.GetSOHost().GetSOStateCtr(ctx, nil)
	if err != nil {
		return err
	}
	defer relSoStateCtr()

	var current *sobject.SOState
	for {
		next, err := soStateCtr.WaitValueChange(ctx, current, nil)
		if err != nil {
			return err
		}
		current = next

		pendingOps := current.GetOps()
		if len(pendingOps) == 0 {
			if !watch {
				return nil
			}
			continue
		}

		snap := sobject.NewSOStateParticipantHandle(
			s.tkr.a.le,
			s.tkr.a.sfs,
			s.GetSharedObjectID(),
			current,
			s.privKey,
			s.localPid,
		)

		nextRoot, rejectedOps, acceptedOps, err := snap.ProcessOperations(
			ctx,
			pendingOps,
			func(ctx context.Context, currentStateData []byte, ops []*sobject.SOOperationInner) (*[]byte, []*sobject.SOOperationResult, error) {
				return cb(ctx, snap, currentStateData, ops)
			},
		)
		if err != nil {
			if ctx.Err() != nil {
				return context.Canceled
			}
			if !watch {
				return err
			}
			s.tkr.a.le.WithError(err).Warn("error processing operations")
			continue
		}

		if err := s.host.GetSOHost().UpdateRootState(
			ctx,
			nextRoot,
			s.GetPeerID().String(),
			rejectedOps,
			acceptedOps,
		); err != nil {
			return err
		}

		if !watch {
			return nil
		}
	}
}

// sobjectTracker tracks a SharedObject in the ProviderAccount.
type sobjectTracker struct {
	// a is the provider account
	a *ProviderAccount
	// id is the sobject id
	id string
	// ref is the reference to the shared object, set when instantiating the tracker.
	ref *promise.Promise[*sobject.SharedObjectRef]
	// sobjectProm is the sobject promise container.
	sobjectProm *promise.PromiseContainer[*SharedObject]
	// healthCtr contains the current shared object health snapshot.
	healthCtr *ccontainer.CContainer[*sobject.SharedObjectHealth]
}

// buildSharedObjectTracker builds a new sobjectTracker for a sobject id.
func (a *ProviderAccount) buildSharedObjectTracker(sobjectID string) (keyed.Routine, *sobjectTracker) {
	tracker := &sobjectTracker{
		a:           a,
		id:          sobjectID,
		ref:         promise.NewPromise[*sobject.SharedObjectRef](),
		sobjectProm: promise.NewPromiseContainer[*SharedObject](),
		healthCtr: ccontainer.NewCContainer[*sobject.SharedObjectHealth](
			sobject.NewSharedObjectLoadingHealth(
				sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
			),
		),
	}
	return tracker.executeSharedObjectTracker, tracker
}

// setHealth updates the current shared object health snapshot when tracking is enabled.
func (t *sobjectTracker) setHealth(health *sobject.SharedObjectHealth) {
	if t.healthCtr == nil {
		return
	}
	t.healthCtr.SetValue(health)
}

// executeSharedObjectTracker executes the sobjectTracker for the sobject.
func (t *sobjectTracker) executeSharedObjectTracker(rctx context.Context) (rerr error) {
	// Clear old state if any.
	t.sobjectProm.SetPromise(nil)
	t.setHealth(
		sobject.NewSharedObjectLoadingHealth(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
		),
	)
	defer func() {
		if rerr != nil && rerr != context.Canceled {
			t.setHealth(sobject.BuildSharedObjectHealthFromError(
				sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
				rerr,
			))
			t.sobjectProm.SetResult(nil, rerr)
		}
	}()

	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	le := t.a.le.WithField("sobject-id", t.id)
	le.Debug("mounting sobject")

	// Wait for the ref.
	sobjectRef, err := t.ref.Await(ctx)
	if err != nil {
		return err
	}

	provRef := sobjectRef.GetProviderResourceRef()
	providerID := provRef.GetProviderId()
	providerAccountID := provRef.GetProviderAccountId()
	sharedObjectID := provRef.GetId()

	// Mount block store.
	blkStore, blkStoreRef, err := bstore.ExMountBlockStore(
		ctx,
		t.a.p.b,
		NewBlockStoreRef(
			providerID,
			providerAccountID,
			sobjectRef.GetBlockStoreId(),
		),
		false,
		ctxCancel,
	)
	if err != nil {
		if isTerminalSharedObjectMountError(err) {
			return t.holdTerminalMountError(ctx, err)
		}
		return err
	}
	defer blkStoreRef.Release()

	le.Debug("mounted block store for sobject successfully")

	cloudBlkStore, ok := blkStore.(*BlockStore)
	if !ok {
		return errors.New("unexpected block store type")
	}

	// Extract the session private key. Required for signing operations and decrypting grants.
	sessionCli, sessionPriv, sessionPeerID, err := t.a.getReadySessionClient(ctx)
	if err != nil {
		return err
	}

	verifiedCache, err := t.a.loadVerifiedSOStateCache(ctx, sharedObjectID)
	if err != nil {
		le.WithError(err).Warn("failed to load verified SO state cache")
	}

	// Create cloudSOHost.
	host := newCloudSOHost(
		le,
		sessionCli,
		sharedObjectID,
		t.a.accountID,
		t.a.wsTracker,
		sessionPriv,
		sessionPeerID,
		t.a.sfs,
		verifiedCache,
		func(ctx context.Context, cache *api.VerifiedSOStateCache) error {
			return t.a.writeVerifiedSOStateCache(ctx, sharedObjectID, cache)
		},
		cloudBlkStore.ForceSync,
	)
	host.soHost.SetContext(ctx)

	so := &SharedObject{
		tkr:      t,
		blkStore: blkStore,
		host:     host,
		privKey:  sessionPriv,
		localPid: sessionPeerID,
	}
	if err := t.tryRecoverMissingSharedObjectPeer(
		ctx,
		sobjectRef,
		so,
		sessionCli,
	); err != nil {
		if isTerminalSharedObjectMountError(err) {
			return t.holdTerminalMountError(ctx, err)
		}
		return err
	}

	if err := host.ensureInitialState(ctx, SeedReasonColdSeed); err != nil {
		if ctx.Err() != nil {
			return context.Canceled
		}
		if isTerminalSharedObjectMountError(err) {
			return t.holdTerminalMountError(
				ctx,
				errors.Wrap(err, "initial state pull"),
			)
		}
		return errors.Wrap(err, "initial state pull")
	}
	t.setHealth(
		sobject.NewSharedObjectReadyHealth(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
		),
	)
	t.sobjectProm.SetResult(so, nil)
	defer t.sobjectProm.SetPromise(nil)

	// Execute the cloud host logic, blocks until cancelled.
	return host.Execute(ctx)
}

// holdTerminalMountError delivers a terminal mount error to waiters while
// keeping the keyed routine alive so generic retry backoff does not recreate
// the same broken shared object mount on a timer.
func (t *sobjectTracker) holdTerminalMountError(
	ctx context.Context,
	err error,
) error {
	if t.a != nil {
		t.a.reportClientError(
			ctx,
			clientErrorReportCodeSharedObjectInitialStateRejected,
			clientErrorReportComponentSharedObjectTracker,
			"shared_object",
			t.id,
			err.Error(),
		)
	}
	t.setHealth(
		sobject.NewSharedObjectClosedHealth(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
			sobject.SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_INITIAL_STATE_REJECTED,
			sobject.SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_CONTACT_OWNER,
			err.Error(),
		),
	)
	t.sobjectProm.SetResult(nil, err)
	<-ctx.Done()
	return context.Canceled
}

// MountSharedObject attempts to mount a SharedObject returning the sobject and a release function.
func (a *ProviderAccount) MountSharedObject(ctx context.Context, ref *sobject.SharedObjectRef, released func()) (sobject.SharedObject, func(), error) {
	if err := ref.Validate(); err != nil {
		return nil, nil, err
	}

	sobjectID := ref.GetProviderResourceRef().GetId()
	tkrRef, tkr, _ := a.sobjects.AddKeyRef(sobjectID)

	// Set the ref in the tracker if not set.
	tkr.ref.SetResult(ref, nil)

	// Await the sobject handle to be ready.
	ws, err := tkr.sobjectProm.Await(ctx)
	if err != nil {
		tkrRef.Release()
		return nil, nil, err
	}

	return ws, tkrRef.Release, nil
}

// AccessSharedObjectHealth adds a reference to shared object health by ref.
func (a *ProviderAccount) AccessSharedObjectHealth(
	ctx context.Context,
	ref *sobject.SharedObjectRef,
	released func(),
) (ccontainer.Watchable[*sobject.SharedObjectHealth], func(), error) {
	if err := ref.Validate(); err != nil {
		return nil, nil, err
	}
	sobjectID := ref.GetProviderResourceRef().GetId()
	tkrRef, tkr, _ := a.sobjects.AddKeyRef(sobjectID)
	tkr.ref.SetResult(ref, nil)
	return tkr.healthCtr, func() {
		tkrRef.Release()
		if released != nil {
			released()
		}
	}, nil
}

// CreateSharedObject creates a new shared object with the given details.
func (a *ProviderAccount) CreateSharedObject(ctx context.Context, id string, meta *sobject.SharedObjectMeta, ownerType, ownerID string) (*sobject.SharedObjectRef, error) {
	if err := meta.Validate(); err != nil {
		return nil, err
	}
	ownerType, ownerID = a.normalizeSharedObjectCreateOwner(ownerType, ownerID)
	displayName := getSharedObjectDisplayName(meta)
	objectType := meta.GetBodyType()

	if err := a.sessionClient.CreateSharedObject(
		ctx,
		id,
		displayName,
		objectType,
		ownerType,
		ownerID,
		meta.GetAccountPrivate(),
	); err != nil {
		return nil, err
	}

	// Perform client-side crypto initialization.
	_, sessionPriv, _, err := a.getReadySessionClient(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "session private key not available for crypto init")
	}
	if err := a.initCloudSharedObjectState(ctx, id, sessionPriv); err != nil {
		return nil, errors.Wrap(err, "init shared object state")
	}

	// Register GC hierarchy: gcroot -> sw-provider -> bucket
	if kvVol, ok := a.vol.(kvtx_volume.KvtxVolume); ok {
		if rg := kvVol.GetRefGraph(); rg != nil {
			bstoreID := SobjectBlockStoreID(id)
			bucketID := BlockStoreBucketID(a.accountID, bstoreID)
			providerID := a.p.info.GetProviderId()
			if err := block_gc.RegisterEntityChain(ctx, rg,
				block_gc.NodeGCRoot,
				ProviderIRI(providerID),
				block_gc.BucketIRI(bucketID),
			); err != nil {
				a.le.WithError(err).Warn("failed to register GC chain")
			}
		}
	}

	ref := a.buildSharedObjectRef(id)
	a.SetSharedObjectMetadata(id, &api.SpaceMetadataResponse{
		OwnerType:   ownerType,
		OwnerId:     ownerID,
		DisplayName: displayName,
		ObjectType:  objectType,
	})
	a.cacheSharedObjectListEntry(&sobject.SharedObjectListEntry{
		Ref:    ref.CloneVT(),
		Meta:   meta.CloneVT(),
		Source: "created",
	})
	return ref, nil
}

func (a *ProviderAccount) normalizeSharedObjectCreateOwner(
	ownerType string,
	ownerID string,
) (string, string) {
	if ownerType == sobject.OwnerTypeOrganization {
		return ownerType, ownerID
	}
	if ownerType == sobject.OwnerTypeAccount && ownerID != "" {
		return ownerType, ownerID
	}
	return sobject.OwnerTypeAccount, a.accountID
}

// buildSharedObjectRef constructs a SharedObjectRef for the given shared object ID.
func (a *ProviderAccount) buildSharedObjectRef(id string) *sobject.SharedObjectRef {
	providerID := a.p.info.GetProviderId()
	providerAccountID := a.accountID
	blockStoreID := SobjectBlockStoreID(id)
	return sobject.NewSharedObjectRef(providerID, providerAccountID, id, blockStoreID)
}

// cacheSharedObjectListEntry ensures the cached shared object list contains the
// given entry before the next full refresh arrives from the cloud.
func (a *ProviderAccount) cacheSharedObjectListEntry(
	entry *sobject.SharedObjectListEntry,
) {
	if entry == nil || entry.GetRef() == nil || entry.GetRef().GetProviderResourceRef() == nil {
		return
	}
	soID := entry.GetRef().GetProviderResourceRef().GetId()
	if soID == "" {
		return
	}

	a.soListCtr.SwapValue(func(list *sobject.SharedObjectList) *sobject.SharedObjectList {
		if list == nil {
			return &sobject.SharedObjectList{
				SharedObjects: []*sobject.SharedObjectListEntry{entry.CloneVT()},
			}
		}

		next := list.CloneVT()
		for _, existing := range next.GetSharedObjects() {
			if existing.GetRef().GetProviderResourceRef().GetId() == soID {
				if entry.GetMeta() != nil {
					existing.Meta = entry.GetMeta().CloneVT()
				}
				if entry.GetSource() != "" {
					existing.Source = entry.GetSource()
				}
				return next
			}
		}

		next.SharedObjects = append(next.SharedObjects, entry.CloneVT())
		return next
	})
	a.refreshSelfEnrollmentSummary(context.Background())
}

// PatchSharedObjectListMetadata updates cached list display metadata for an SO.
func (a *ProviderAccount) PatchSharedObjectListMetadata(
	soID string,
	metadata *api.SpaceMetadataResponse,
) {
	meta, ok := sharedObjectListMetaFromMetadata(metadata)
	if soID == "" || !ok {
		return
	}

	a.soListCtr.SwapValue(func(list *sobject.SharedObjectList) *sobject.SharedObjectList {
		if list == nil {
			return nil
		}
		next := list.CloneVT()
		for _, existing := range next.GetSharedObjects() {
			if existing.GetRef().GetProviderResourceRef().GetId() != soID {
				continue
			}
			existing.Meta = meta.CloneVT()
			return next
		}
		return list
	})
	a.refreshSelfEnrollmentSummary(context.Background())
}

// RemoveSharedObjectListEntry removes a deleted shared object from the cached list.
func (a *ProviderAccount) RemoveSharedObjectListEntry(
	soID string,
) {
	if soID == "" {
		return
	}
	a.soListCtr.SwapValue(func(list *sobject.SharedObjectList) *sobject.SharedObjectList {
		if list == nil {
			return nil
		}
		next := list.CloneVT()
		out := next.SharedObjects[:0]
		changed := false
		for _, entry := range next.GetSharedObjects() {
			if entry.GetRef().GetProviderResourceRef().GetId() == soID {
				changed = true
				continue
			}
			out = append(out, entry)
		}
		if !changed {
			return list
		}
		next.SharedObjects = out
		return next
	})
	a.refreshSelfEnrollmentSummary(context.Background())
}

func sharedObjectListMetaFromMetadata(
	metadata *api.SpaceMetadataResponse,
) (*sobject.SharedObjectMeta, bool) {
	if metadata == nil {
		return nil, false
	}
	switch metadata.GetObjectType() {
	case space.SpaceBodyType:
		meta, err := space.NewSharedObjectMeta(metadata.GetDisplayName())
		return meta, err == nil
	case s4wave_org.OrgBodyType:
		return s4wave_org.NewOrgSharedObjectMeta(metadata.GetDisplayName()), true
	default:
		return nil, false
	}
}

// getSharedObjectDisplayName extracts the display name from a space SO's metadata.
func getSharedObjectDisplayName(meta *sobject.SharedObjectMeta) string {
	if meta.GetBodyType() != space.SpaceBodyType {
		return ""
	}

	spaceMeta := &space.SpaceSoMeta{}
	if err := spaceMeta.UnmarshalVT(meta.GetBodyMeta()); err != nil {
		return ""
	}

	return spaceMeta.GetName()
}

// initCloudSharedObjectState performs the client-side crypto initialization
// for a newly created shared object. Generates a random XChaCha20 key, builds
// the initial root, signs it, creates grants, and POSTs the state to the server.
func (a *ProviderAccount) initCloudSharedObjectState(ctx context.Context, sharedObjectID string, localPriv crypto.PrivKey) error {
	le := a.le.WithField("sobject-id", sharedObjectID)
	return initializeCloudSharedObjectState(
		ctx,
		a.sessionClient,
		le,
		a.accountID,
		sharedObjectID,
		localPriv,
		a.sfs,
	)
}

// DeleteSharedObject deletes the shared object with the given ID.
func (a *ProviderAccount) DeleteSharedObject(ctx context.Context, id string) error {
	le := a.le.WithField("sobject-id", id)
	data, err := a.sessionClient.doDelete(ctx, path.Join("/api/sobject", id, "delete"), SeedReasonMutation)
	if err != nil {
		return errors.Wrap(err, "delete shared object")
	}
	var resp api.DeleteSObjectResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return errors.Wrap(err, "unmarshal delete shared object response")
	}

	// Remove GC edges: sw-provider -> bucket (marks orphans as unreferenced)
	if kvVol, ok := a.vol.(kvtx_volume.KvtxVolume); ok {
		if rg := kvVol.GetRefGraph(); rg != nil {
			bstoreID := SobjectBlockStoreID(id)
			bucketID := BlockStoreBucketID(a.accountID, bstoreID)
			bucketIRI := block_gc.BucketIRI(bucketID)
			providerID := a.p.info.GetProviderId()
			gcOps := block_gc.NewGCStoreOps(a.vol, rg)
			if err := gcOps.RemoveGCRef(ctx,
				block_gc.NodeGCRoot,
				bucketIRI,
			); err != nil {
				le.WithError(err).Warn("failed to remove GC root ref")
			}
			if err := gcOps.RemoveGCRef(ctx,
				ProviderIRI(providerID),
				bucketIRI,
			); err != nil {
				le.WithError(err).Warn("failed to remove GC ref")
			}
			if stats, err := block_gc.NewCollector(rg, a.vol, nil).
				Collect(ctx); err != nil {
				le.WithError(err).Warn("GC collect after delete failed")
			} else if stats != nil && stats.NodesSwept > 0 {
				le.Infof("GC swept %d nodes after space delete", stats.NodesSwept)
			}
		}
	}

	a.DeleteSharedObjectMetadata(id)
	a.RemoveSharedObjectListEntry(id)
	if a.sobjects != nil {
		a.sobjects.RemoveKey(id)
	}
	return nil
}

// AccessSharedObjectList adds a reference to the list of shared objects and returns the container.
func (a *ProviderAccount) AccessSharedObjectList(ctx context.Context, released func()) (ccontainer.Watchable[*sobject.SharedObjectList], func(), error) {
	ref := a.soListRc.AddRef(nil)
	return a.soListCtr, ref.Release, nil
}

// hasSOListAccess checks if the subscription status allows SO list access.
func hasSOListAccess(
	subStatus s4wave_provider_spacewave.BillingStatus,
) bool {
	switch subStatus {
	case s4wave_provider_spacewave.BillingStatus_BillingStatus_ACTIVE,
		s4wave_provider_spacewave.BillingStatus_BillingStatus_TRIALING,
		s4wave_provider_spacewave.BillingStatus_BillingStatus_PAST_DUE,
		s4wave_provider_spacewave.BillingStatus_BillingStatus_CANCELED:
		return true
	default:
		return false
	}
}

// RefreshSharedObjectList invalidates and reloads the current shared object list snapshot.
func (a *ProviderAccount) RefreshSharedObjectList(ctx context.Context) error {
	if !a.hasSharedObjectListAccess() {
		a.soListCtr.SetValue(&sobject.SharedObjectList{})
		a.refreshSelfEnrollmentSummary(ctx)
		return nil
	}
	prev := a.soListCtr.GetValue()
	a.invalidateSharedObjectList()
	if err := a.EnsureSharedObjectListLoaded(ctx); err != nil {
		return err
	}
	if prev == nil {
		return nil
	}
	next := a.soListCtr.GetValue()
	if prev != next {
		return nil
	}
	_, err := a.soListCtr.WaitValueChange(ctx, prev, nil)
	return err
}

// HasCachedSharedObject returns true when the cached SO list already contains
// the given shared object ID.
func (a *ProviderAccount) HasCachedSharedObject(soID string) bool {
	if soID == "" {
		return false
	}
	list := a.soListCtr.GetValue()
	if list == nil {
		return false
	}
	for _, entry := range list.GetSharedObjects() {
		if entry.GetRef().GetProviderResourceRef().GetId() == soID {
			return true
		}
	}
	return false
}

// fetchSharedObjectList fetches the shared object list from the server and updates the persistent container.
func (a *ProviderAccount) fetchSharedObjectList(ctx context.Context) error {
	listData, err := a.sessionClient.ListSharedObjects(ctx)
	if err != nil {
		return err
	}

	list := &sobject.SharedObjectList{}
	if err := list.UnmarshalVT(listData); err != nil {
		return errors.Wrap(err, "unmarshal shared object list")
	}

	// The cloud response omits provider_id since the server is not aware of the
	// client's configured provider identifier. Fill it in so ref.Validate()
	// succeeds downstream (e.g. MountSharedObject in the self-rejoin sweep).
	providerID := a.p.info.GetProviderId()
	for _, entry := range list.GetSharedObjects() {
		prr := entry.GetRef().GetProviderResourceRef()
		if prr != nil && prr.GetProviderId() == "" {
			prr.ProviderId = providerID
		}
	}

	a.soListCtr.SetValue(list)
	a.refreshSelfEnrollmentSummary(ctx)
	return nil
}

// SobjectBlockStoreID returns the block store ID for a shared object.
// Block stores backing a shared object share the shared object's ULID
// verbatim; no prefix is added.
func SobjectBlockStoreID(soID string) string {
	return soID
}

// AddParticipant adds a participant to the shared object.
func (s *SharedObject) AddParticipant(ctx context.Context, targetPeerIDStr string, targetPub crypto.PubKey, role sobject.SOParticipantRole, entityID string) (*sobject.SOGrant, error) {
	if err := sobject.ValidateSOParticipantRole(role, false); err != nil {
		return nil, err
	}

	relLock, err := s.host.writeMu.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer relLock()

	cli, err := s.getReadyWriteSessionClient(ctx)
	if err != nil {
		return nil, err
	}
	for attempt := range maxWriteRetries {
		state, currentCfg, epochs, err := s.loadLatestConfigState(ctx)
		if err != nil {
			return nil, err
		}
		var (
			participantExists      bool
			participantNeedsUpdate bool
			participantIdx         int
		)
		for i, p := range currentCfg.GetParticipants() {
			if p.GetPeerId() == targetPeerIDStr {
				participantExists = true
				participantIdx = i
				if p.GetRole() != role {
					participantNeedsUpdate = true
				}
				if p.GetEntityId() == "" && entityID != "" {
					participantNeedsUpdate = true
				}
				break
			}
		}
		epoch := currentEpochWithFallback(state, epochs)
		grantExists := soGrantSliceHasPeerID(state.GetRootGrants(), targetPeerIDStr)
		if !grantExists && epoch != nil {
			grantExists = soGrantSliceHasPeerID(epoch.GetGrants(), targetPeerIDStr)
		}
		if participantExists && !participantNeedsUpdate && grantExists {
			return nil, nil
		}

		localPeerIDStr := s.localPid.String()
		localGrant := findSOGrantByPeerID(state.GetRootGrants(), localPeerIDStr)
		if localGrant == nil && epoch != nil {
			localGrant = findSOGrantByPeerID(epoch.GetGrants(), localPeerIDStr)
		}
		if localGrant == nil {
			return nil, errors.New("local grant not found")
		}

		grantInner, err := localGrant.DecryptInnerData(s.privKey, s.GetSharedObjectID())
		if err != nil {
			return nil, errors.Wrap(err, "decrypt local grant")
		}

		var entry *sobject.SOConfigChange
		var entryData []byte
		if !participantExists || participantNeedsUpdate {
			nextCfg := currentCfg.CloneVT()
			nextParticipant := &sobject.SOParticipantConfig{
				PeerId:   targetPeerIDStr,
				Role:     role,
				EntityId: entityID,
			}
			if participantExists {
				currentParticipant := currentCfg.GetParticipants()[participantIdx]
				if nextParticipant.GetEntityId() == "" {
					nextParticipant.EntityId = currentParticipant.GetEntityId()
				}
				nextCfg.Participants[participantIdx] = nextParticipant
			} else {
				nextCfg.Participants = append(nextCfg.Participants, nextParticipant)
			}
			entry, err = sobject.BuildSOConfigChange(
				currentCfg,
				nextCfg,
				sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT,
				s.privKey,
				nil,
			)
			if err != nil {
				return nil, errors.Wrap(err, "build config change")
			}
			entryData, err = entry.MarshalVT()
			if err != nil {
				return nil, errors.Wrap(err, "marshal config change")
			}
		}

		var grant *sobject.SOGrant
		if !grantExists {
			grant, err = sobject.EncryptSOGrant(
				s.privKey,
				targetPub,
				s.GetSharedObjectID(),
				grantInner,
			)
			if err != nil {
				return nil, errors.Wrap(err, "encrypt grant for target peer")
			}

			if epoch == nil {
				epoch = &sobject.SOKeyEpoch{
					Epoch:      sobject.CurrentEpochNumber(epochs),
					SeqnoStart: state.GetRoot().GetInnerSeqno(),
					Grants:     append([]*sobject.SOGrant(nil), state.GetRootGrants()...),
				}
			}
			if epoch.GetSeqnoStart() == 0 {
				epoch.SeqnoStart = state.GetRoot().GetInnerSeqno()
			}
			epoch.Grants = append(epoch.GetGrants(), grant)
		}

		var postedEpoch *sobject.SOKeyEpoch
		if !grantExists {
			postedEpoch = epoch
		}

		recoveryCfg, err := recoveryConfigSnapshot(currentCfg, entry)
		if err != nil {
			return nil, errors.Wrap(err, "build recovery config snapshot")
		}
		recoveryKeyEpoch := sobject.CurrentEpochNumber(epochs)
		if postedEpoch != nil {
			recoveryKeyEpoch = postedEpoch.GetEpoch()
		}
		recoveryEnvelopes, err := s.buildRecoveryEnvelopesForConfig(
			ctx,
			cli,
			state,
			epoch,
			recoveryCfg,
			recoveryKeyEpoch,
		)
		if err != nil {
			var missingErr *missingRecoveryKeypairsError
			if !errors.As(err, &missingErr) || missingErr.entityID != entityID ||
				sobject.CanReadState(readableParticipantRoleForEntity(currentCfg, entityID)) {
				return nil, err
			}
			recoveryEnvelopes, err = s.buildRecoveryEnvelopesForConfig(
				ctx,
				cli,
				state,
				epoch,
				recoveryConfigWithoutEntity(recoveryCfg, entityID),
				recoveryKeyEpoch,
			)
			if err != nil {
				return nil, err
			}
		}

		if entryData != nil {
			if err := cli.PostConfigState(
				ctx,
				s.GetSharedObjectID(),
				entryData,
				nil,
				postedEpoch,
				recoveryEnvelopes,
			); err != nil {
				var ce *cloudError
				if !errors.As(err, &ce) || ce.StatusCode != 409 || attempt+1 == maxWriteRetries {
					return nil, err
				}
				continue
			}
		} else if postedEpoch != nil {
			if err := cli.PostKeyEpoch(
				ctx,
				s.GetSharedObjectID(),
				epoch,
				recoveryEnvelopes,
			); err != nil {
				var ce *cloudError
				if !errors.As(err, &ce) || ce.StatusCode != 409 || attempt+1 == maxWriteRetries {
					return nil, err
				}
				continue
			}
		}
		if entry != nil {
			if err := s.host.applyConfigMutation(ctx, entry, nil, postedEpoch); err != nil {
				return nil, err
			}
		} else if postedEpoch != nil {
			s.host.applyKeyEpoch(ctx, postedEpoch)
		}

		return grant, nil
	}

	return nil, errors.New("add participant failed after max retries due to config conflicts")
}

func (s *SharedObject) getReadyWriteSessionClient(ctx context.Context) (*SessionClient, error) {
	cli, _, _, err := s.tkr.a.getReadySessionClient(ctx)
	if err != nil {
		return nil, err
	}
	return cli, nil
}

func currentEpochWithFallback(state *sobject.SOState, epochs []*sobject.SOKeyEpoch) *sobject.SOKeyEpoch {
	currentEpoch := sobject.CurrentEpochNumber(epochs)
	for _, epoch := range epochs {
		if epoch.GetEpoch() == currentEpoch {
			return epoch.CloneVT()
		}
	}
	if state == nil || state.GetRoot() == nil {
		return nil
	}
	return &sobject.SOKeyEpoch{
		Epoch:      currentEpoch,
		SeqnoStart: state.GetRoot().GetInnerSeqno(),
		Grants:     append([]*sobject.SOGrant(nil), state.GetRootGrants()...),
	}
}

func soGrantSliceHasPeerID(grants []*sobject.SOGrant, peerID string) bool {
	for _, grant := range grants {
		if grant.GetPeerId() == peerID {
			return true
		}
	}
	return false
}

func findSOGrantByPeerID(grants []*sobject.SOGrant, peerID string) *sobject.SOGrant {
	for _, grant := range grants {
		if grant.GetPeerId() == peerID {
			return grant
		}
	}
	return nil
}

func configWithConfigChangeHash(
	entry *sobject.SOConfigChange,
) (*sobject.SharedObjectConfig, error) {
	if entry == nil || entry.GetConfig() == nil {
		return nil, errors.New("config change entry is required")
	}
	hash, err := sobject.HashSOConfigChange(entry)
	if err != nil {
		return nil, err
	}
	cfg := entry.GetConfig().CloneVT()
	cfg.ConfigChainSeqno = entry.GetConfigSeqno()
	cfg.ConfigChainHash = hash
	return cfg, nil
}

func recoveryConfigSnapshot(
	currentCfg *sobject.SharedObjectConfig,
	entry *sobject.SOConfigChange,
) (*sobject.SharedObjectConfig, error) {
	if entry == nil {
		if currentCfg == nil {
			return nil, errors.New("current config is required")
		}
		return currentCfg, nil
	}
	return configWithConfigChangeHash(entry)
}

func recoveryConfigWithoutEntity(
	cfg *sobject.SharedObjectConfig,
	entityID string,
) *sobject.SharedObjectConfig {
	if cfg == nil || entityID == "" {
		return cfg
	}
	next := cfg.CloneVT()
	participants := next.GetParticipants()
	filtered := make([]*sobject.SOParticipantConfig, 0, len(participants))
	for _, participant := range participants {
		if participant.GetEntityId() == entityID {
			continue
		}
		filtered = append(filtered, participant)
	}
	next.Participants = filtered
	return next
}

func (s *SharedObject) decryptLocalGrantInner(
	state *sobject.SOState,
	epoch *sobject.SOKeyEpoch,
) (*sobject.SOGrantInner, error) {
	localGrant := findSOGrantByPeerID(state.GetRootGrants(), s.localPid.String())
	if localGrant == nil && epoch != nil {
		localGrant = findSOGrantByPeerID(epoch.GetGrants(), s.localPid.String())
	}
	if localGrant == nil {
		return nil, errors.New("local grant not found")
	}
	grantInner, err := localGrant.DecryptInnerData(
		s.privKey,
		s.GetSharedObjectID(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "decrypt local grant")
	}
	return grantInner, nil
}

func (s *SharedObject) buildRecoveryEnvelopesForConfig(
	ctx context.Context,
	cli *SessionClient,
	state *sobject.SOState,
	epoch *sobject.SOKeyEpoch,
	recoveryCfg *sobject.SharedObjectConfig,
	recoveryKeyEpoch uint64,
) ([]*sobject.SOEntityRecoveryEnvelope, error) {
	grantInner, err := s.decryptLocalGrantInner(state, epoch)
	if err != nil {
		return nil, err
	}
	recoveryEnvelopes, err := buildSORecoveryEnvelopes(
		ctx,
		cli,
		s.GetSharedObjectID(),
		recoveryCfg,
		recoveryKeyEpoch,
		grantInner,
	)
	if err != nil {
		return nil, errors.Wrap(err, "build recovery envelopes")
	}
	return recoveryEnvelopes, nil
}

// cloneSOGrants deep-copies a slice of SOGrant messages.
func cloneSOGrants(grants []*sobject.SOGrant) []*sobject.SOGrant {
	cloned := make([]*sobject.SOGrant, 0, len(grants))
	for _, grant := range grants {
		cloned = append(cloned, grant.CloneVT())
	}
	return cloned
}

// cloneSOKeyEpochs deep-copies a slice of SOKeyEpoch messages.
func cloneSOKeyEpochs(epochs []*sobject.SOKeyEpoch) []*sobject.SOKeyEpoch {
	cloned := make([]*sobject.SOKeyEpoch, 0, len(epochs))
	for _, epoch := range epochs {
		cloned = append(cloned, epoch.CloneVT())
	}
	return cloned
}

// mergeSOKeyEpochs appends a new epoch to the cloned epoch list, closing the previous epoch's seqno range.
func mergeSOKeyEpochs(epochs []*sobject.SOKeyEpoch, next *sobject.SOKeyEpoch) []*sobject.SOKeyEpoch {
	cloned := cloneSOKeyEpochs(epochs)
	if next == nil {
		return cloned
	}

	next = next.CloneVT()
	if next.GetEpoch() > 0 {
		prevEpoch := next.GetEpoch() - 1
		for _, epoch := range cloned {
			if epoch.GetEpoch() == prevEpoch && epoch.GetSeqnoEnd() == 0 {
				epoch.SeqnoEnd = next.GetSeqnoStart() - 1
				break
			}
		}
	}

	for i, epoch := range cloned {
		if epoch.GetEpoch() == next.GetEpoch() {
			cloned[i] = next
			return cloned
		}
	}
	return append(cloned, next)
}

func (s *SharedObject) loadLatestConfigState(ctx context.Context) (*sobject.SOState, *sobject.SharedObjectConfig, []*sobject.SOKeyEpoch, error) {
	if err := s.host.pullState(ctx, SeedReasonReconnect); err != nil {
		return nil, nil, nil, errors.Wrap(err, "pull latest SO state")
	}

	state, err := s.host.GetSOHost().GetHostState(ctx)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "get current SO state")
	}

	currentCfg := state.GetConfig()
	if currentCfg == nil {
		currentCfg = &sobject.SharedObjectConfig{}
	} else {
		currentCfg = currentCfg.CloneVT()
	}

	var lastHash []byte
	var lastSeqno uint64
	var epochs []*sobject.SOKeyEpoch
	s.host.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		lastHash = append([]byte(nil), s.host.lastConfigChainHash...)
		lastSeqno = s.host.verifiedConfigChainSeqno
		epochs = cloneSOKeyEpochs(s.host.keyEpochs)
	})

	currentHash := currentCfg.GetConfigChainHash()
	currentSeqno := currentCfg.GetConfigChainSeqno()
	if shouldSyncVerifiedConfigChain(currentHash, currentSeqno, lastHash, lastSeqno) {
		if err := s.host.syncConfigChain(ctx, currentHash); err != nil {
			return nil, nil, nil, errors.Wrap(err, "sync config chain")
		}
		s.host.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			lastHash = append([]byte(nil), s.host.lastConfigChainHash...)
			lastSeqno = s.host.verifiedConfigChainSeqno
			epochs = cloneSOKeyEpochs(s.host.keyEpochs)
		})
	}
	if len(lastHash) != 0 {
		currentCfg.ConfigChainHash = append([]byte(nil), lastHash...)
		currentCfg.ConfigChainSeqno = lastSeqno
	}

	return state, currentCfg, epochs, nil
}

func cloneSOInvites(invites []*sobject.SOInvite) []*sobject.SOInvite {
	cloned := make([]*sobject.SOInvite, 0, len(invites))
	for _, invite := range invites {
		cloned = append(cloned, invite.CloneVT())
	}
	return cloned
}

func (s *SharedObject) applyInviteMutation(
	ctx context.Context,
	signerPrivKey crypto.PrivKey,
	changeType sobject.SOConfigChangeType,
	updateFn func(invites []*sobject.SOInvite) ([]*sobject.SOInvite, error),
) error {
	relLock, err := s.host.writeMu.Lock(ctx)
	if err != nil {
		return err
	}
	defer relLock()

	cli, err := s.getReadyWriteSessionClient(ctx)
	if err != nil {
		return err
	}
	for attempt := range maxWriteRetries {
		state, currentCfg, epochs, err := s.loadLatestConfigState(ctx)
		if err != nil {
			return err
		}
		epoch := currentEpochWithFallback(state, epochs)

		nextInvites, err := updateFn(cloneSOInvites(state.GetInvites()))
		if err != nil {
			return err
		}

		entry, err := sobject.BuildSOConfigChange(
			currentCfg,
			currentCfg,
			changeType,
			signerPrivKey,
			nil,
		)
		if err != nil {
			return errors.Wrap(err, "build config change")
		}
		entryData, err := entry.MarshalVT()
		if err != nil {
			return errors.Wrap(err, "marshal config change")
		}

		recoveryCfg, err := recoveryConfigSnapshot(currentCfg, entry)
		if err != nil {
			return errors.Wrap(err, "build recovery config snapshot")
		}
		recoveryEnvelopes, err := s.buildRecoveryEnvelopesForConfig(
			ctx,
			cli,
			state,
			epoch,
			recoveryCfg,
			sobject.CurrentEpochNumber(epochs),
		)
		if err != nil {
			return err
		}

		if err := cli.PostConfigState(
			ctx,
			s.GetSharedObjectID(),
			entryData,
			nextInvites,
			nil,
			recoveryEnvelopes,
		); err != nil {
			var ce *cloudError
			if !errors.As(err, &ce) || ce.StatusCode != 409 || attempt+1 == maxWriteRetries {
				return err
			}
			continue
		}
		if err := s.host.applyConfigMutation(ctx, entry, nextInvites, nil); err != nil {
			return err
		}
		return nil
	}

	return errors.New("invite mutation failed after max retries due to config conflicts")
}

// CreateSOInviteOp creates a cloud-backed invite and returns the signed invite message.
func (s *SharedObject) CreateSOInviteOp(
	ctx context.Context,
	ownerPrivKey crypto.PrivKey,
	role sobject.SOParticipantRole,
	providerID string,
	targetPeerID string,
	maxUses uint32,
	expiresAt *timestamppb.Timestamp,
) (*sobject.SOInviteMessage, error) {
	msg, invite, err := sobject.BuildSOInviteMessage(
		s.GetSharedObjectID(),
		ownerPrivKey,
		role,
		providerID,
		targetPeerID,
		maxUses,
		expiresAt,
	)
	if err != nil {
		return nil, err
	}
	if err := s.CreateInvite(ctx, ownerPrivKey, invite); err != nil {
		return nil, errors.Wrap(err, "store invite on-chain")
	}
	return msg, nil
}

// CreateInvite creates a cloud-backed invite.
func (s *SharedObject) CreateInvite(ctx context.Context, signerPrivKey crypto.PrivKey, invite *sobject.SOInvite) error {
	if invite == nil {
		return errors.New("invite is nil")
	}
	if invite.GetInviteId() == "" {
		return errors.New("invite_id is required")
	}
	if len(invite.GetTokenHash()) == 0 {
		return errors.New("token_hash is required")
	}

	return s.applyInviteMutation(
		ctx,
		signerPrivKey,
		sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_INVITE,
		func(invites []*sobject.SOInvite) ([]*sobject.SOInvite, error) {
			for _, existing := range invites {
				if existing.GetInviteId() == invite.GetInviteId() {
					return nil, errors.New("invite_id already exists")
				}
			}
			return append(invites, invite.CloneVT()), nil
		},
	)
}

// RevokeInvite revokes a cloud-backed invite.
func (s *SharedObject) RevokeInvite(ctx context.Context, signerPrivKey crypto.PrivKey, inviteID string) error {
	if inviteID == "" {
		return errors.New("invite_id is required")
	}

	return s.applyInviteMutation(
		ctx,
		signerPrivKey,
		sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REVOKE_INVITE,
		func(invites []*sobject.SOInvite) ([]*sobject.SOInvite, error) {
			for _, invite := range invites {
				if invite.GetInviteId() != inviteID {
					continue
				}
				if invite.GetRevoked() {
					return nil, errors.New("invite is already revoked")
				}
				invite.Revoked = true
				return invites, nil
			}
			return nil, errors.New("invite not found")
		},
	)
}

// IncrementInviteUses increments uses for a cloud-backed invite.
func (s *SharedObject) IncrementInviteUses(ctx context.Context, signerPrivKey crypto.PrivKey, inviteID string) error {
	if inviteID == "" {
		return errors.New("invite_id is required")
	}

	return s.applyInviteMutation(
		ctx,
		signerPrivKey,
		sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_INCREMENT_INVITE_USES,
		func(invites []*sobject.SOInvite) ([]*sobject.SOInvite, error) {
			for _, invite := range invites {
				if invite.GetInviteId() != inviteID {
					continue
				}
				if err := sobject.ValidateInviteUsable(invite); err != nil {
					return nil, err
				}
				invite.Uses++
				return invites, nil
			}
			return nil, errors.New("invite not found")
		},
	)
}

// RemoveParticipant removes a participant from the shared object.
// Returns true if the participant was found and removed.
func (s *SharedObject) RemoveParticipant(ctx context.Context, targetPeerIDStr string) (bool, error) {
	return s.RemoveParticipantWithRevocation(ctx, targetPeerIDStr, nil)
}

// RemoveParticipantWithRevocation removes a participant from the shared object.
// Returns true if the participant was found and removed.
func (s *SharedObject) RemoveParticipantWithRevocation(
	ctx context.Context,
	targetPeerIDStr string,
	revInfo *sobject.SORevocationInfo,
) (bool, error) {
	if targetPeerIDStr == "" {
		return false, errors.New("target peer id is required")
	}

	relLock, err := s.host.writeMu.Lock(ctx)
	if err != nil {
		return false, err
	}
	defer relLock()

	cli, err := s.getReadyWriteSessionClient(ctx)
	if err != nil {
		return false, err
	}
	for attempt := range maxWriteRetries {
		state, currentCfg, epochs, err := s.loadLatestConfigState(ctx)
		if err != nil {
			return false, err
		}

		var (
			foundParticipant bool
			nextParticipants []*sobject.SOParticipantConfig
		)
		for _, participant := range currentCfg.GetParticipants() {
			if participant.GetPeerId() == targetPeerIDStr {
				foundParticipant = true
				continue
			}
			nextParticipants = append(nextParticipants, participant.CloneVT())
		}
		if !foundParticipant {
			return false, nil
		}

		nextCfg := currentCfg.CloneVT()
		nextCfg.Participants = nextParticipants
		entry, err := sobject.BuildSOConfigChange(
			currentCfg,
			nextCfg,
			sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT,
			s.privKey,
			revInfo,
		)
		if err != nil {
			return false, errors.Wrap(err, "build config change")
		}
		entryData, err := entry.MarshalVT()
		if err != nil {
			return false, errors.Wrap(err, "marshal config change")
		}

		epoch := currentEpochWithFallback(state, epochs)
		var grantRemoved bool
		if epoch != nil {
			filteredGrants := make([]*sobject.SOGrant, 0, len(epoch.GetGrants()))
			for _, grant := range epoch.GetGrants() {
				if grant.GetPeerId() == targetPeerIDStr {
					grantRemoved = true
					continue
				}
				filteredGrants = append(filteredGrants, grant.CloneVT())
			}
			epoch.Grants = filteredGrants
		}

		var postedEpoch *sobject.SOKeyEpoch
		if grantRemoved {
			if epoch == nil {
				return false, errors.New("current key epoch missing for participant removal")
			}
			postedEpoch = epoch
		}

		recoveryCfg, err := recoveryConfigSnapshot(currentCfg, entry)
		if err != nil {
			return false, errors.Wrap(err, "build recovery config snapshot")
		}
		recoveryKeyEpoch := sobject.CurrentEpochNumber(epochs)
		if postedEpoch != nil {
			recoveryKeyEpoch = postedEpoch.GetEpoch()
		}
		recoveryEnvelopes, err := s.buildRecoveryEnvelopesForConfig(
			ctx,
			cli,
			state,
			epoch,
			recoveryCfg,
			recoveryKeyEpoch,
		)
		if err != nil {
			return false, err
		}

		if err := cli.PostConfigState(
			ctx,
			s.GetSharedObjectID(),
			entryData,
			nil,
			postedEpoch,
			recoveryEnvelopes,
		); err != nil {
			var ce *cloudError
			if !errors.As(err, &ce) || ce.StatusCode != 409 || attempt+1 == maxWriteRetries {
				return false, err
			}
			continue
		}
		if err := s.host.applyConfigMutation(ctx, entry, nil, postedEpoch); err != nil {
			return false, err
		}
		return true, nil
	}

	return false, errors.New("remove participant failed after max retries due to config conflicts")
}

// GetSOHost returns the SOHost for invite operations.
func (s *SharedObject) GetSOHost() *sobject.SOHost {
	return s.host.GetSOHost()
}

// GetPrivKey returns the private key for signing invite messages.
func (s *SharedObject) GetPrivKey() crypto.PrivKey {
	return s.privKey
}

// GetProviderID returns the provider identifier for the invite message.
func (s *SharedObject) GetProviderID() string {
	return s.tkr.a.GetProviderID()
}

// _ is a type assertion
var (
	_ sobject.SharedObjectProvider = ((*ProviderAccount)(nil))
	_ sobject.SharedObject         = ((*SharedObject)(nil))
	_ sobject.InviteHost           = ((*SharedObject)(nil))
)
