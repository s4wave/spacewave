package provider_spacewave

import (
	"bytes"
	"cmp"
	"context"
	"path"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
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
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	"github.com/s4wave/spacewave/db/volume"
	kvtx_volume "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_org "github.com/s4wave/spacewave/sdk/org"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// SharedObject implements the sobject interface attached to sobjectTracker.
type SharedObject struct {
	// tkr retains the provider account and shared object state.
	tkr *sobjectTracker
	// blkStore stores the shared object blocks.
	blkStore bstore.BlockStore
	// host runs the cloud-backed shared object.
	host *cloudSOHost
	// privKey authenticates the local participant.
	privKey crypto.PrivKey
	// localPid identifies the local participant.
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

// GetBackingVolume returns the volume that owns this SharedObject's storage.
func (s *SharedObject) GetBackingVolume() volume.Volume {
	return s.tkr.a.vol
}

// AccessLocalStateStore accesses a kvtx ops for a local state store with the
// given ID.
func (s *SharedObject) AccessLocalStateStore(ctx context.Context, storeID string, released func()) (kvtx.Store, func(), error) {
	if s.tkr.a.objStore == nil {
		return nil, nil, errors.New("account object store not ready")
	}
	store := kvtx_prefixer.NewPrefixer(s.tkr.a.objStore, []byte("so-local/"+s.tkr.id+"/"+storeID+"/"))
	if released == nil {
		return store, func() {}, nil
	}
	stop := context.AfterFunc(ctx, released)
	return store, func() { stop() }, nil
}

// AccessPublicationRetention retains completion proofs for cloud-bound graphs.
func (s *SharedObject) AccessPublicationRetention(ctx context.Context) (kvtx.Store, func(), error) {
	store, ok := s.blkStore.(*BlockStore)
	if !ok || store.retention == nil {
		return nil, nil, errors.New("cloud block retention unavailable")
	}
	release, err := store.retention.mu.Lock(ctx)
	if err != nil {
		return nil, nil, err
	}
	return store.retention.store, release, nil
}

// GetSharedObjectState returns a snapshot of the shared object state.
func (s *SharedObject) GetSharedObjectState(ctx context.Context) (sobject.SharedObjectStateSnapshot, error) {
	// Wait for the host's first accepted state.
	stateCtr, relStateCtr, err := s.host.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer relStateCtr()
	soState, err := stateCtr.WaitValue(ctx, nil)
	if err != nil {
		return nil, err
	}

	// Project it through the host's participant handle.
	return s.host.newSnapshot(soState), nil
}

// AccessSharedObjectState adds a reference to the state and returns the state
// container.
func (s *SharedObject) AccessSharedObjectState(ctx context.Context, released func()) (ccontainer.Watchable[sobject.SharedObjectStateSnapshot], func(), error) {
	return s.host.AccessSharedObjectSnapshot(), func() {}, nil
}

// AccessSharedObjectHealth retains the provider's health watch for this mount.
func (s *SharedObject) AccessSharedObjectHealth(ctx context.Context, released func()) (ccontainer.Watchable[*sobject.SharedObjectHealth], func(), error) {
	ref, err := s.tkr.ref.Await(ctx)
	if err != nil {
		return nil, nil, err
	}
	return s.tkr.a.AccessSharedObjectHealth(ctx, ref, released)
}

// SetRejectedEdits shows edits in the health as this device's edits that replay
// no longer applies.
func (s *SharedObject) SetRejectedEdits(edits []*sobject.SORejectedEdit) {
	if s.tkr.healthCtr == nil {
		return
	}
	s.tkr.healthCtr.SwapValue(func(health *sobject.SharedObjectHealth) *sobject.SharedObjectHealth {
		return health.WithRejectedEdits(edits)
	})
}

// SetCheckpointMismatch shows mismatch in the health as a checkpoint whose
// World differs from the World this device replayed.
func (s *SharedObject) SetCheckpointMismatch(mismatch *sobject.SOCheckpointMismatch) {
	if s.tkr.healthCtr == nil {
		return
	}
	s.tkr.healthCtr.SwapValue(func(health *sobject.SharedObjectHealth) *sobject.SharedObjectHealth {
		return health.WithCheckpointMismatch(mismatch)
	})
}

// SetUnorderedCount shows in the health that n operations wait for the
// sequencer to place them.
func (s *SharedObject) SetUnorderedCount(n uint32) {
	if s.tkr.healthCtr == nil {
		return
	}
	s.tkr.healthCtr.SwapValue(func(health *sobject.SharedObjectHealth) *sobject.SharedObjectHealth {
		return health.WithUnorderedCount(n)
	})
}

// QueueOperation signs op as the session peer and adds it to the operation
// set. It returns the operation's local ID once the state holding it is
// durably accepted for publication.
func (s *SharedObject) QueueOperation(ctx context.Context, op []byte) (string, error) {
	localID, _, err := s.host.GetSOHost().AddLocalOperation(ctx, s.tkr.a.le, s.tkr.a.sfs, s.privKey, op)
	return localID, err
}

// sobjectTracker tracks a SharedObject in the ProviderAccount.
type sobjectTracker struct {
	// a is the provider account.
	a *ProviderAccount
	// id is the shared object ID.
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

// setHealth updates the current shared object health snapshot when tracking is
// enabled.
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
		cloudBlkStore.syncer,
	)
	host.refreshBlockManifest = cloudBlkStore.RefreshRemote
	if host.syncer != nil {
		host.syncer.setPublication(host, host.pending)
		defer host.syncer.setPublication(host, nil)
	}
	host.blockManifestSequence = cloudBlkStore.RemoteSequence
	host.stateObserved = func(state *sobject.SOState) {
		checkpoint, err := state.GetCheckpointInner()
		if err != nil || checkpoint == nil {
			return
		}
		t.a.setSyncTelemetryAcceptedCheckpoint(
			sobjectRef.GetBlockStoreId(),
			sharedObjectID,
			checkpoint.GetHeight(),
		)
	}
	so := &SharedObject{
		tkr:      t,
		blkStore: blkStore,
		host:     host,
		privKey:  sessionPriv,
		localPid: sessionPeerID,
	}
	defer t.sobjectProm.SetPromise(nil)
	err = host.execute(ctx, func(ctx context.Context) error {
		if err := t.tryRecoverMissingSharedObjectPeer(ctx, sobjectRef, so, sessionCli); err != nil {
			return err
		}
		t.setHealth(sobject.NewSharedObjectReadyHealth(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
		))
		t.sobjectProm.SetResult(so, nil)
		return nil
	})
	if isTerminalSharedObjectMountError(err) {
		return t.holdTerminalMountError(ctx, err)
	}
	return err
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
	t.sobjectProm.SetResult(nil, sobject.NewSharedObjectHealthError(t.healthCtr.GetValue(), err))
	<-ctx.Done()
	return context.Canceled
}

// MountSharedObject attempts to mount a SharedObject returning the sobject and
// a release function.
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
	// Resolve the owner and display fields from the metadata.
	if err := meta.Validate(); err != nil {
		return nil, err
	}
	ownerType, ownerID = a.normalizeSharedObjectCreateOwner(ownerType, ownerID)
	displayName := getSharedObjectDisplayName(meta)
	objectType := meta.GetBodyType()

	// Create the object in the Cloud catalog.
	cli, sessionPriv, _, err := a.getReadySessionClient(ctx)
	if err != nil {
		return nil, err
	}
	if err := cli.CreateSharedObject(
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

	// Initialize the object with the same authorized Session that created it.
	username, err := a.getUsername(ctx)
	if err != nil {
		return nil, err
	}
	if err := initializeCloudSharedObjectState(ctx, cli, a.le.WithField("sobject-id", id), a.accountID, username, id, sessionPriv, a.sfs, objectType == space.SpaceBodyType); err != nil {
		return nil, errors.Wrap(err, "init shared object state")
	}
	ref := a.buildSharedObjectRef(id)

	// Creation enters the cloud catalog before its root is initialized. An
	// enrollment sweep can mount that empty state while the Session WebSocket
	// is still connecting, so publish the completed write to that existing host.
	if _, mounted := a.sobjects.GetKey(id); mounted {
		object, release, err := a.MountSharedObject(ctx, ref, nil)
		if err != nil {
			return nil, err
		}
		defer release()
		host := object.(*SharedObject).host
		if host.stateCtr.GetValue().GetCheckpoint() == nil {
			if err := host.pullState(ctx, SeedReasonMutation); err != nil {
				return nil, errors.Wrap(err, "publish initialized shared object")
			}
		}
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

	// Cache the new object's metadata and list entry.
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

// buildSharedObjectRef constructs a SharedObjectRef for the given shared object
// ID.
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

// RemoveSharedObjectListEntry removes a deleted shared object from the cached
// list.
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

// settleCreatedSharedObjectListEntry clears the created source of soID's
// cached entry, so the next cloud list snapshot decides whether it stays.
func (a *ProviderAccount) settleCreatedSharedObjectListEntry(soID string) {
	a.soListCtr.SwapValue(func(list *sobject.SharedObjectList) *sobject.SharedObjectList {
		for i, entry := range list.GetSharedObjects() {
			if entry.GetRef().GetProviderResourceRef().GetId() != soID || entry.GetSource() != "created" {
				continue
			}
			next := list.CloneVT()
			next.SharedObjects[i].Source = ""
			return next
		}
		return list
	})
}

// sharedObjectListMetaFromMetadata builds typed metadata for a supported
// object.
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

// getSharedObjectDisplayName extracts the display name from a space SO's
// metadata.
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

// DeleteSharedObject deletes the shared object with the given ID.
func (a *ProviderAccount) DeleteSharedObject(ctx context.Context, id string) error {
	le := a.le.WithField("sobject-id", id)
	cli, _, _, err := a.getReadySessionClient(ctx)
	if err != nil {
		return err
	}
	data, err := cli.doDelete(ctx, path.Join("/api/sobject", id, "delete"), SeedReasonMutation)
	if err != nil {
		return errors.Wrap(err, "delete shared object")
	}
	var resp api.DeleteSObjectResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return errors.Wrap(err, "unmarshal delete shared object response")
	}

	a.DeleteSharedObjectMetadata(id)
	a.RemoveSharedObjectListEntry(id)
	if a.sobjects != nil {
		a.sobjects.RemoveKey(id)
	}
	a.removeSharedObjectGCRefs(ctx, id, le)
	a.triggerGCCleanup()
	return nil
}

// AccessSharedObjectList adds a reference to the list of shared objects and
// returns the container.
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

// RefreshSharedObjectList fetches a fresh shared object list snapshot.
// Returns once the fetched snapshot is stored.
func (a *ProviderAccount) RefreshSharedObjectList(ctx context.Context) error {
	if !a.hasSharedObjectListAccess() {
		if _, err := a.GetAccountState(ctx); err != nil {
			return err
		}
		if !a.hasSharedObjectListAccess() {
			a.soListCtr.SetValue(&sobject.SharedObjectList{})
			a.refreshSelfEnrollmentSummary(ctx)
			return nil
		}
	}
	return a.fetchSharedObjectList(ctx)
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

// fetchSharedObjectList fetches the shared object list from the server and
// updates the persistent container.
func (a *ProviderAccount) fetchSharedObjectList(ctx context.Context) error {
	cli := a.currentSessionClient()
	if cli == nil {
		return errors.New("Session is not available to list shared objects")
	}
	listData, err := cli.ListSharedObjects(ctx)
	if err != nil {
		return err
	}

	list, err := DecodeSharedObjectList(listData, a.p.info.GetProviderId())
	if err != nil {
		return err
	}

	a.soListCtr.SetValue(mergeSharedObjectListSnapshot(list, a.soListCtr.GetValue()))
	a.refreshSelfEnrollmentSummary(ctx)
	return nil
}

// DecodeSharedObjectList decodes a cloud shared object list response.
//
// The cloud response omits provider_id since the server is not aware of the
// client's configured provider identifier. Entries missing it are filled in
// with providerID so ref.Validate() succeeds downstream.
func DecodeSharedObjectList(data []byte, providerID string) (*sobject.SharedObjectList, error) {
	list := &sobject.SharedObjectList{}
	if err := list.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal shared object list")
	}
	for _, entry := range list.GetSharedObjects() {
		prr := entry.GetRef().GetProviderResourceRef()
		if prr != nil && prr.GetProviderId() == "" {
			prr.ProviderId = providerID
		}
	}
	return list, nil
}

func mergeSharedObjectListSnapshot(
	snapshot *sobject.SharedObjectList,
	cached *sobject.SharedObjectList,
) *sobject.SharedObjectList {
	if cached == nil {
		return snapshot
	}
	next := snapshot.CloneVT()
	if next == nil {
		next = &sobject.SharedObjectList{}
	}
	seen := make(map[string]bool, len(next.GetSharedObjects()))
	for _, entry := range next.GetSharedObjects() {
		soID := entry.GetRef().GetProviderResourceRef().GetId()
		if soID != "" {
			seen[soID] = true
		}
	}
	for _, entry := range cached.GetSharedObjects() {
		if entry.GetSource() != "created" {
			continue
		}
		soID := entry.GetRef().GetProviderResourceRef().GetId()
		if soID == "" || seen[soID] {
			continue
		}
		next.SharedObjects = append(next.SharedObjects, entry.CloneVT())
		seen[soID] = true
	}
	return next
}

// SobjectBlockStoreID returns the block store ID for a shared object.
// Block stores backing a shared object share the shared object's ULID
// verbatim; no prefix is added.
func SobjectBlockStoreID(soID string) string {
	return soID
}

// AddParticipant adds a participant to the shared object. username is the
// provider username of entityID; it fills a missing username on an existing
// participant.
func (s *SharedObject) AddParticipant(
	ctx context.Context,
	targetPeerIDStr string,
	targetPub crypto.PubKey,
	role sobject.SOParticipantRole,
	entityID string,
	username string,
) (*sobject.SOGrant, error) {
	// Reject roles a participant add may not grant.
	if err := sobject.ValidateSOParticipantRole(role, false); err != nil {
		return nil, err
	}

	// Serialize writes to this object and take a write session.
	relLock, err := s.host.writeMu.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer relLock()
	cli, err := s.getReadyWriteSessionClient(ctx)
	if err != nil {
		return nil, err
	}

	// Write the change against the latest config, retrying on a conflict.
	for attempt := range maxWriteRetries {
		// Load the latest config and key epochs.
		state, currentCfg, epochs, err := s.loadLatestConfigState(ctx)
		if err != nil {
			return nil, err
		}

		// Find the participant and whether it lacks the role, entity or username.
		participantIdx := slices.IndexFunc(currentCfg.GetParticipants(), func(p *sobject.SOParticipantConfig) bool {
			return p.GetPeerId() == targetPeerIDStr
		})
		participantExists := participantIdx >= 0
		participantNeedsUpdate := false
		if participantExists {
			current := currentCfg.GetParticipants()[participantIdx]
			participantNeedsUpdate = current.GetRole() != sobject.MaxSOParticipantRole(current.GetRole(), role) ||
				(current.GetEntityId() == "" && entityID != "") ||
				(current.GetUsername() == "" && username != "")
		}

		// Return early when the participant and its grant are current.
		epoch := currentEpochWithFallback(state, epochs)
		if epoch == nil {
			return nil, errors.New("current key epoch missing")
		}
		grantExists := epoch.FindGrant(targetPeerIDStr) != nil
		if participantExists && !participantNeedsUpdate && grantExists {
			return nil, nil
		}

		// Recover the read key from the local grant.
		localGrant := epoch.FindGrant(s.localPid.String())
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
			nextRole := role
			if participantExists {
				nextRole = sobject.MaxSOParticipantRole(currentCfg.GetParticipants()[participantIdx].GetRole(), role)
			}
			nextParticipant := &sobject.SOParticipantConfig{
				PeerId:   targetPeerIDStr,
				Role:     nextRole,
				EntityId: entityID,
				Username: username,
			}
			if participantExists {
				currentParticipant := currentCfg.GetParticipants()[participantIdx]
				if nextParticipant.GetEntityId() == "" {
					nextParticipant.EntityId = currentParticipant.GetEntityId()
				}
				if nextParticipant.GetUsername() == "" {
					nextParticipant.Username = currentParticipant.GetUsername()
				}
				nextCfg.Participants[participantIdx] = nextParticipant
			} else {
				nextCfg.Participants = append(nextCfg.Participants, nextParticipant)
			}
			entry, err = sobject.BuildSOConfigChange(
				s.GetSharedObjectID(),
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
		recoveryKeyEpoch := epoch.GetEpoch()
		recoveryEnvelopes, err := s.buildRecoveryEnvelopesForConfig(
			ctx,
			cli,
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

// currentEpochWithFallback returns a copy of the newest key epoch held by
// state or listed in epochs, which ascend by epoch.
func currentEpochWithFallback(state *sobject.SOState, epochs []*sobject.SOKeyEpoch) *sobject.SOKeyEpoch {
	current := state.CurrentKeyEpoch()
	if n := len(epochs); n != 0 && (current == nil || epochs[n-1].GetEpoch() > current.GetEpoch()) {
		current = epochs[n-1]
	}
	return current.CloneVT()
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

// decryptLocalGrantInner decrypts the session peer's grant in epoch.
func (s *SharedObject) decryptLocalGrantInner(epoch *sobject.SOKeyEpoch) (*sobject.SOGrantInner, error) {
	// Find and decrypt the session peer's grant.
	localGrant := epoch.FindGrant(s.localPid.String())
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

// buildRecoveryEnvelopesForConfig seals the local grant of epoch into recovery
// envelopes for the entities of recoveryCfg.
func (s *SharedObject) buildRecoveryEnvelopesForConfig(
	ctx context.Context,
	cli *SessionClient,
	epoch *sobject.SOKeyEpoch,
	recoveryCfg *sobject.SharedObjectConfig,
	recoveryKeyEpoch uint64,
) ([]*sobject.SOEntityRecoveryEnvelope, error) {
	// Seal the local grant for the config's entities.
	grantInner, err := s.decryptLocalGrantInner(epoch)
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

// cloneVTSlice deep-copies a slice of VT-clonable proto messages.
func cloneVTSlice[T interface{ CloneVT() T }](items []T) []T {
	cloned := make([]T, 0, len(items))
	for _, item := range items {
		cloned = append(cloned, item.CloneVT())
	}
	return cloned
}

// mergeSOKeyEpochs returns a copy of epochs, which ascend by epoch, with next
// added or replacing the epoch of the same number.
func mergeSOKeyEpochs(epochs []*sobject.SOKeyEpoch, next *sobject.SOKeyEpoch) []*sobject.SOKeyEpoch {
	// Replace the epoch of the same number, or insert next in order.
	cloned := cloneVTSlice(epochs)
	if next == nil {
		return cloned
	}
	i, ok := slices.BinarySearchFunc(cloned, next.GetEpoch(), func(e *sobject.SOKeyEpoch, n uint64) int {
		return cmp.Compare(e.GetEpoch(), n)
	})
	if ok {
		cloned[i] = next.CloneVT()
		return cloned
	}
	return slices.Insert(cloned, i, next.CloneVT())
}

func (s *SharedObject) loadLatestConfigState(ctx context.Context) (*sobject.SOState, *sobject.SharedObjectConfig, []*sobject.SOKeyEpoch, error) {
	if err := s.host.pullState(ctx, SeedReasonReconnect); err != nil {
		return nil, nil, nil, errors.Wrap(err, "pull latest SO state")
	}

	state, err := s.host.GetSOHost().GetHostState(ctx)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "get current SO state")
	}

	currentCfg := &sobject.SharedObjectConfig{}
	if cfg := state.GetConfig(); cfg != nil {
		currentCfg = cfg.CloneVT()
	}

	var lastHash []byte
	var lastSeqno uint64
	var epochs []*sobject.SOKeyEpoch
	s.host.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		lastHash = bytes.Clone(s.host.lastConfigChainHash)
		lastSeqno = s.host.verifiedConfigChainSeqno
		epochs = cloneVTSlice(s.host.keyEpochs)
	})

	currentHash := currentCfg.GetConfigChainHash()
	currentSeqno := currentCfg.GetConfigChainSeqno()
	if shouldSyncVerifiedConfigChain(currentHash, currentSeqno, lastHash, lastSeqno) {
		if err := s.host.syncConfigChain(ctx, currentHash); err != nil {
			return nil, nil, nil, errors.Wrap(err, "sync config chain")
		}
		s.host.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			lastHash = bytes.Clone(s.host.lastConfigChainHash)
			lastSeqno = s.host.verifiedConfigChainSeqno
			epochs = cloneVTSlice(s.host.keyEpochs)
		})
	}
	if len(lastHash) != 0 {
		currentCfg.ConfigChainHash = bytes.Clone(lastHash)
		currentCfg.ConfigChainSeqno = lastSeqno
	}

	return state, currentCfg, epochs, nil
}

// applyInviteMutation writes the config change that updateFn makes to the
// invite list, retrying when another writer advances the config.
func (s *SharedObject) applyInviteMutation(
	ctx context.Context,
	signerPrivKey crypto.PrivKey,
	changeType sobject.SOConfigChangeType,
	updateFn func(invites []*sobject.SOInvite) ([]*sobject.SOInvite, error),
) error {
	// Serialize with local writes and connect a write session.
	relLock, err := s.host.writeMu.Lock(ctx)
	if err != nil {
		return err
	}
	defer relLock()
	cli, err := s.getReadyWriteSessionClient(ctx)
	if err != nil {
		return err
	}

	// Write against the latest config, retrying when another writer advances it.
	for attempt := range maxWriteRetries {
		state, currentCfg, epochs, err := s.loadLatestConfigState(ctx)
		if err != nil {
			return err
		}
		epoch := currentEpochWithFallback(state, epochs)

		nextInvites, err := updateFn(cloneVTSlice(state.GetInvites()))
		if err != nil {
			return err
		}

		entry, err := sobject.BuildSOConfigChange(
			s.GetSharedObjectID(),
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
			epoch,
			recoveryCfg,
			epoch.GetEpoch(),
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

// CreateSOInviteOp creates a cloud-backed invite and returns the signed invite
// message.
func (s *SharedObject) CreateSOInviteOp(
	ctx context.Context,
	ownerPrivKey crypto.PrivKey,
	providerID string,
	terms *sobject.SOInvite,
) (*sobject.SOInviteMessage, error) {
	msg, invite, err := sobject.BuildSOInviteMessage(s.GetSharedObjectID(), ownerPrivKey, providerID, terms)
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
	removed, err := s.RemoveParticipantsWithRevocation(ctx, []string{targetPeerIDStr}, revInfo)
	return len(removed) != 0, err
}

// RemoveParticipantsWithRevocation removes the target peers from the shared
// object in one signed config change and returns the peers it removed. Peers
// that do not participate are ignored.
func (s *SharedObject) RemoveParticipantsWithRevocation(
	ctx context.Context,
	targetPeerIDs []string,
	revInfo *sobject.SORevocationInfo,
) ([]string, error) {
	var removed []string
	err := s.retryConfigConflicts(ctx, func() error {
		var err error
		removed, err = sobject.RemoveSOParticipants(ctx, s.GetSOHost(), targetPeerIDs, s.privKey, revInfo)
		return err
	})
	return removed, err
}

// retryConfigConflicts runs a configuration write against the latest cloud
// state, retrying when another writer advances the configuration first. The
// cloud reports a conflict with 409; a local head advanced between reading
// the config and locking it fails verification with a head mismatch.
func (s *SharedObject) retryConfigConflicts(ctx context.Context, write func() error) error {
	for attempt := range maxWriteRetries {
		if _, _, _, err := s.loadLatestConfigState(ctx); err != nil {
			return err
		}
		err := write()
		if err == nil || !isConfigConflict(err) || attempt+1 == maxWriteRetries {
			return err
		}
	}
	return nil
}

// SetRosterDropped drops exactly the writers in dropped from the trimming
// roster, signed by the local peer against the latest cloud configuration.
func (s *SharedObject) SetRosterDropped(ctx context.Context, dropped []string) (bool, error) {
	var changed bool
	err := s.retryConfigConflicts(ctx, func() error {
		var err error
		changed, err = sobject.SetSORoster(ctx, s.GetSOHost(), dropped, s.privKey)
		return err
	})
	return changed, err
}

// SetSequencer appoints peerID as the sequencer, or selects Merge when peerID
// is empty, signed by the local peer against the latest cloud configuration.
func (s *SharedObject) SetSequencer(ctx context.Context, peerID string) (bool, error) {
	var changed bool
	err := s.retryConfigConflicts(ctx, func() error {
		var err error
		changed, err = sobject.SetSOSequencer(ctx, s.GetSOHost(), peerID, s.privKey)
		return err
	})
	return changed, err
}

// SetControl changes who controls the shared object, signed or agreed to by
// the local peer against the latest cloud configuration.
func (s *SharedObject) SetControl(ctx context.Context, control sobject.SOControl) error {
	return s.retryConfigConflicts(ctx, func() error {
		return sobject.SetSOControl(ctx, s.GetSOHost(), control, s.privKey)
	})
}

// ApproveConfigChange agrees, as the local peer, to a change another voter
// asked the group for.
func (s *SharedObject) ApproveConfigChange(ctx context.Context, hash []byte) error {
	return sobject.ApproveSOConfigChange(ctx, s.GetSOHost(), hash, s.privKey)
}

// GetProviderSequencer returns the peer ID Spacewave Cloud signs the shared
// object's order with.
func (s *SharedObject) GetProviderSequencer(ctx context.Context) (string, error) {
	return s.host.client.GetSOSequencer(ctx, s.tkr.id)
}

// isConfigConflict reports whether a config write lost a race with another
// writer and may be rebuilt against the current head.
func isConfigConflict(err error) bool {
	var ce *cloudError
	if errors.As(err, &ce) && ce.StatusCode == 409 {
		return true
	}
	return errors.Is(err, sobject.ErrConfigChainHeadMismatch)
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
	_ sobject.SharedObjectHealthAccessor = (*SharedObject)(nil)
	_ sobject.ReplayReporter             = (*SharedObject)(nil)
	_ sobject.ProviderSequencer          = (*SharedObject)(nil)
	_ sobject.SharedObjectProvider       = (*ProviderAccount)(nil)
	_ sobject.SharedObject               = (*SharedObject)(nil)
	_ sobject.InviteHost                 = (*SharedObject)(nil)
	_ sobject.RosterHost                 = (*SharedObject)(nil)
	_ sobject.SequencerHost              = (*SharedObject)(nil)
	_ sobject.ControlHost                = (*SharedObject)(nil)
)
