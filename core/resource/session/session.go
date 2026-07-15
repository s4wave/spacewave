package resource_session

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"slices"
	"sync"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/ulid"
	b58 "github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_state "github.com/s4wave/spacewave/bldr/resource/state"
	"github.com/s4wave/spacewave/core/cdn"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/resource/session/mountedresource"
	resource_sobject "github.com/s4wave/spacewave/core/resource/sobject"
	resource_space "github.com/s4wave/spacewave/core/resource/space"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_invite "github.com/s4wave/spacewave/core/sobject/invite"
	"github.com/s4wave/spacewave/core/space"
	space_world "github.com/s4wave/spacewave/core/space/world"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/volume"
	kvtx_volume "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/db/world"
	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/util/confparse"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	s4wave_status "github.com/s4wave/spacewave/sdk/status"
	"github.com/sirupsen/logrus"
)

// SessionResource wraps a core session for resource access.
//
// The lifecycle context ctx scopes any background work owned by this
// resource. It is canceled by Close when the mount is released.
type SessionResource struct {
	le           *logrus.Entry
	b            bus.Bus
	mux          srpc.Invoker
	session      session.Session
	hostPluginID string
	transferMgr  transferManager

	// ctx is the lifecycle context for background work owned by this
	// resource. Canceled by Close.
	ctx       context.Context
	ctxCancel context.CancelFunc

	localPairingMu sync.Mutex
	localPairing   *localPairingState

	// cdnMtx guards cdnRootChangedRelease. The session no longer owns the
	// CDN SharedObject itself (that moved to core/resource/cdn.Registry);
	// it only forwards cdn-root-changed notifications to the root singleton
	// via a hook set by the enclosing mount path.
	cdnMtx sync.Mutex
	// cdnRootChangedRelease releases the provider-level subscription that
	// forwards cdn-root-changed frames to the hook. nil when the session's
	// provider does not emit the event (e.g. local-only) or when no hook
	// has been wired.
	cdnRootChangedRelease func()
	// cdnLookup maps a shared object id to the process-scoped CDN-backed
	// SharedObject when the id corresponds to a registered CDN Space.
	// Returns nil for ids that are not CDN Spaces. The synthesized
	// SharedObjectMeta surfaces BodyType so downstream dispatch in
	// MountSharedObjectBody can route to the CDN pipeline. Wired from
	// the enclosing mount path to the root CDN registry so
	// MountSharedObject can return the anonymous CDN singleton without
	// going through the per-session SO list (the CDN Space is filtered
	// out of that list intentionally).
	cdnLookup func(sharedObjectID string) (sobject.SharedObject, *sobject.SharedObjectMeta)
}

type acceptedCloudInviteAccount interface {
	RefreshSharedObjectList(context.Context) error
}

// NewSessionResource creates a new SessionResource.
func NewSessionResource(le *logrus.Entry, b bus.Bus, sess session.Session) *SessionResource {
	return NewSessionResourceWithHostPluginID(le, b, sess, "")
}

// NewSessionResourceWithHostPluginID creates a new SessionResource with the
// plugin id that owns the resource root.
func NewSessionResourceWithHostPluginID(
	le *logrus.Entry,
	b bus.Bus,
	sess session.Session,
	hostPluginID string,
) *SessionResource {
	return NewSessionResourceWithHostPluginIDAndRecoveryStatus(
		le,
		b,
		sess,
		hostPluginID,
		nil,
	)
}

// NewSessionResourceWithHostPluginIDAndRecoveryStatus creates a new
// SessionResource with an explicit recovery status registry.
func NewSessionResourceWithHostPluginIDAndRecoveryStatus(
	le *logrus.Entry,
	b bus.Bus,
	sess session.Session,
	hostPluginID string,
	recoveryStatusRegistry *RecoveryStatusRegistry,
) *SessionResource {
	ctx, ctxCancel := context.WithCancel(context.Background())
	sessResource := &SessionResource{
		le:           le,
		b:            b,
		session:      sess,
		hostPluginID: hostPluginID,
		ctx:          ctx,
		ctxCancel:    ctxCancel,
	}

	statusRes := NewStatusResourceWithSession(
		b,
		sess,
		recoveryStatusRegistry.GetSessionRecoveryStatusCtr(sess),
	)
	registrations := []func(srpc.Mux) error{
		func(mux srpc.Mux) error {
			return s4wave_session.SRPCRegisterSessionResourceService(mux, sessResource)
		},
		func(mux srpc.Mux) error {
			return s4wave_status.SRPCRegisterSystemStatusService(mux, statusRes)
		},
	}

	// Register provider-specific session services.
	switch acc := sess.GetProviderAccount().(type) {
	case *provider_spacewave.ProviderAccount:
		sw := NewSpacewaveSessionResource(sessResource, le, b, sess, acc)
		registrations = append(registrations, func(mux srpc.Mux) error {
			return s4wave_session.SRPCRegisterSpacewaveSessionResourceService(mux, sw)
		})
	case *provider_local.ProviderAccount:
		localRes := NewLocalSessionResource(b, sess)
		registrations = append(registrations, func(mux srpc.Mux) error {
			return s4wave_session.SRPCRegisterLocalSessionResourceService(mux, localRes)
		})
	}

	sessResource.mux = resource_server.NewResourceMux(registrations...)
	return sessResource
}

// GetMux returns the rpc mux.
func (r *SessionResource) GetMux() srpc.Invoker {
	return r.mux
}

// Close releases resources owned by this SessionResource. Callers that
// wrap a SessionResource via resource_server.AddResource should invoke
// Close from the release callback so the lifecycle context is canceled
// and any provider-level subscriptions are released.
func (r *SessionResource) Close() {
	r.cdnMtx.Lock()
	release := r.cdnRootChangedRelease
	r.cdnRootChangedRelease = nil
	r.cdnMtx.Unlock()
	if release != nil {
		release()
	}
	r.ctxCancel()
}

// SetCdnRootChangedHook wires a callback that fires when the session's
// provider account delivers a cdn-root-changed WS frame. Wired by the
// enclosing mount path to the root CdnInstance.Refresh() so pushes on the
// upstream CDN root wake up the process-scoped singleton.
//
// No-op when the session's provider is not spacewave (local-only never
// receives cdn-root-changed). Safe to call once per SessionResource; a
// second call releases the second subscription to preserve single ownership.
func (r *SessionResource) SetCdnRootChangedHook(hook func(spaceID string)) {
	if hook == nil {
		return
	}
	acc, ok := r.session.GetProviderAccount().(*provider_spacewave.ProviderAccount)
	if !ok {
		return
	}
	release := acc.RegisterCdnRootChangedCallback(hook)
	r.cdnMtx.Lock()
	if r.cdnRootChangedRelease != nil {
		r.cdnMtx.Unlock()
		release()
		return
	}
	r.cdnRootChangedRelease = release
	r.cdnMtx.Unlock()
}

// SetCdnLookup wires a lookup from shared object id to the process-scoped
// CDN-backed SharedObject and its synthesized metadata. Consulted by
// MountSharedObject before falling back to the per-session SO list so CDN
// Spaces (which are filtered out of that list) remain mountable by ULID.
//
// The lookup must return (nil, nil) for ids that are not CDN Spaces. Safe
// to call once per SessionResource; subsequent calls replace the previous
// lookup.
func (r *SessionResource) SetCdnLookup(
	lookup func(sharedObjectID string) (sobject.SharedObject, *sobject.SharedObjectMeta),
) {
	r.cdnLookup = lookup
}

// AccessStateAtom accesses a session-scoped state atom resource.
func (r *SessionResource) AccessStateAtom(
	ctx context.Context,
	req *s4wave_session.AccessSessionStateAtomRequest,
) (*s4wave_session.AccessSessionStateAtomResponse, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	storeID := req.GetStoreId()
	if storeID == "" {
		storeID = resource_state.DefaultStateAtomStoreID
	}

	store, err := r.session.AccessStateAtomStore(ctx, storeID)
	if err != nil {
		return nil, err
	}

	stateResource := resource_state.NewStateAtomResource(store)
	id, err := resourceCtx.AddResource(stateResource.GetMux(), func() {})
	if err != nil {
		return nil, err
	}

	return &s4wave_session.AccessSessionStateAtomResponse{ResourceId: id}, nil
}

// GetSessionInfo returns information about this session.
func (r *SessionResource) GetSessionInfo(ctx context.Context, req *s4wave_session.GetSessionInfoRequest) (*s4wave_session.GetSessionInfoResponse, error) {
	resp := &s4wave_session.GetSessionInfoResponse{
		SessionRef: r.session.GetSessionRef(),
		PeerId:     r.session.GetPeerId().String(),
	}
	resp.CryptoInfo = r.buildCryptoInfo()
	return resp, nil
}

// buildCryptoInfo extracts cheap crypto identity from the session.
func (r *SessionResource) buildCryptoInfo() *s4wave_session.SessionCryptoInfo {
	info := &s4wave_session.SessionCryptoInfo{}
	pubKey, err := r.session.GetPeerId().ExtractPublicKey()
	if err != nil {
		return info
	}
	info.KeyType = bifrost_crypto.KeyType_name[int32(pubKey.Type())]
	raw, err := pubKey.Raw()
	if err == nil {
		info.PublicKeyBase58 = b58.Encode(raw)
	}
	pemData, err := confparse.MarshalPublicKeyPEM(pubKey)
	if err == nil {
		info.PublicKeyPem = string(pemData)
	}
	return info
}

func (r *SessionResource) addSharedObjectResource(
	ctx context.Context,
	resourceCtx resource_server.ResourceClientContext,
	soRef *sobject.SharedObjectRef,
	meta *sobject.SharedObjectMeta,
) (*s4wave_session.MountSharedObjectResponse, error) {
	if err := soRef.Validate(); err != nil {
		return nil, err
	}

	mountedSo, mountedSoRef, err := sobject.ExMountSharedObject(ctx, r.session.GetBus(), soRef, false, nil)
	if err != nil {
		return nil, err
	}

	soResource := resource_sobject.NewSharedObjectResourceWithHostPluginID(
		r.le,
		r.b,
		mountedSo,
		meta,
		soRef,
		r.session.GetPeerId().String(),
		r.hostPluginID,
	)
	id, err := resourceCtx.AddResource(soResource.GetMux(), mountedSoRef.Release)
	if err != nil {
		mountedSoRef.Release()
		return nil, err
	}

	return &s4wave_session.MountSharedObjectResponse{
		ResourceId:       id,
		SharedObjectMeta: meta,
		PeerId:           mountedSo.GetPeerID().String(),
		SharedObjectId:   mountedSo.GetSharedObjectID(),
		BlockStoreId:     mountedSo.GetBlockStore().GetID(),
		HashType:         mountedSo.GetBlockStore().GetHashType(),
	}, nil
}

// CreateSpace creates a new space within the ProviderAccount with the Session.
func (r *SessionResource) CreateSpace(ctx context.Context, req *s4wave_session.CreateSpaceRequest) (*s4wave_session.CreateSpaceResponse, error) {
	// Create the new shared object metadata
	soId := ulid.NewULID()
	soMeta, err := space.NewSharedObjectMeta(req.GetSpaceName())
	if err != nil {
		return nil, err
	}

	// Get the provider account feature for shared objects.
	providerAcc := r.session.GetProviderAccount()
	soFeature, err := sobject.GetSharedObjectProviderAccountFeature(ctx, providerAcc)
	if err != nil {
		return nil, err
	}

	// Default owner = caller's account when unspecified.
	ownerType := req.GetOwnerType()
	ownerID := req.GetOwnerId()
	if ownerType == "" && ownerID == "" {
		ownerType = sobject.OwnerTypeAccount
		ownerID = r.session.GetSessionRef().GetProviderResourceRef().GetProviderAccountId()
	}

	// Create the new shared object.
	soRef, err := soFeature.CreateSharedObject(ctx, soId, soMeta, ownerType, ownerID)
	if err != nil {
		return nil, err
	}

	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Initialize the space world immediately so later readers and writers do not
	// block waiting for the first owner mount to seed the head state. Keep the
	// mounted resources attached to the client so first-use flows do not need to
	// immediately remount the same SharedObject by id.
	mountedSpace, spaceBodyRef, err := space.ExMountSpaceSoBody(ctx, r.b, soRef, false, nil)
	if err != nil {
		return nil, err
	}
	spaceResource := resource_space.NewSpaceResourceWithSessionPeerIDAndHostPluginID(
		r.le,
		r.b,
		mountedSpace.GetSharedObjectBody(),
		r.session.GetPeerId().String(),
		r.hostPluginID,
	)
	spaceBodyID, err := mountedresource.Add(
		resourceCtx,
		spaceResource.GetMux(),
		spaceResource,
		spaceBodyRef.Release,
	)
	if err != nil {
		spaceBodyRef.Release()
		return nil, err
	}

	spaceWorld, err := spaceResource.AccessWorld(ctx, &s4wave_space.AccessWorldRequest{})
	if err != nil {
		resourceCtx.ReleaseResource(spaceBodyID)
		return nil, err
	}
	spaceWorldID := spaceWorld.GetResourceId()

	mountedSharedObject, err := r.addSharedObjectResource(ctx, resourceCtx, soRef, soMeta)
	if err != nil {
		resourceCtx.ReleaseResource(spaceWorldID)
		resourceCtx.ReleaseResource(spaceBodyID)
		return nil, err
	}

	return &s4wave_session.CreateSpaceResponse{
		SharedObjectRef:            soRef,
		SharedObjectMeta:           soMeta,
		MountedSharedObject:        mountedSharedObject,
		SharedObjectBodyResourceId: spaceBodyID,
		SpaceWorldResourceId:       spaceWorldID,
	}, nil
}

// WatchResourcesList returns the list of resources the session has access to.
func (r *SessionResource) WatchResourcesList(
	req *s4wave_session.WatchResourcesListRequest,
	strm s4wave_session.SRPCSessionResourceService_WatchResourcesListStream,
) error {
	ctx, ctxCancel := context.WithCancel(strm.Context())
	defer ctxCancel()

	providerAcc := r.session.GetProviderAccount()
	soProvider, err := sobject.GetSharedObjectProviderAccountFeature(ctx, providerAcc)
	if err != nil {
		return err
	}

	soListWatchable, relSoList, err := soProvider.AccessSharedObjectList(ctx, ctxCancel)
	if err != nil {
		return err
	}
	defer relSoList()

	if soListWatchable.GetValue() == nil {
		if err := strm.Send(&s4wave_session.WatchResourcesListResponse{}); err != nil {
			return err
		}
	}

	listCh := make(chan *sobject.SharedObjectList)
	listErrCh := make(chan error, 1)
	projectionCh := make(chan resourcesListProjectionEvent)
	workers := make(map[string]resourcesListProjectionWorker)
	objectTypes := make(map[string]string)
	var workerGeneration uint64
	var wg sync.WaitGroup

	wg.Go(func() {
		var current *sobject.SharedObjectList
		for {
			next, err := soListWatchable.WaitValueChange(ctx, current, nil)
			if err != nil {
				select {
				case listErrCh <- err:
				case <-ctx.Done():
				}
				return
			}
			current = next
			select {
			case listCh <- next:
			case <-ctx.Done():
				return
			}
		}
	})
	defer func() {
		ctxCancel()
		for _, worker := range workers {
			worker.cancel()
		}
		wg.Wait()
	}()

	var currentList []*space.SpaceSoListEntry
	pendingInitial := make(map[string]uint64)
	initialProjectionChanged := false
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-listErrCh:
			return err
		case soList := <-listCh:
			list, err := space.FilterSharedObjectList(soList.GetSharedObjects(), func(ent *sobject.SharedObjectListEntry, err error) error {
				return nil
			})
			if err != nil {
				return err
			}
			currentList = filterOutCdnSpace(list)

			for _, worker := range workers {
				worker.cancel()
			}
			clear(workers)
			clear(pendingInitial)
			initialProjectionChanged = false

			if req.GetIncludeIndexObjectTypes() {
				present := make(map[string]struct{}, len(currentList))
				for _, entry := range currentList {
					ref := entry.GetEntry().GetRef()
					id := ref.GetProviderResourceRef().GetId()
					if id == "" || ref == nil {
						continue
					}
					present[id] = struct{}{}
					workerGeneration++
					workerCtx, workerCancel := context.WithCancel(ctx)
					workers[id] = resourcesListProjectionWorker{
						cancel:     workerCancel,
						generation: workerGeneration,
					}
					pendingInitial[id] = workerGeneration
					wg.Add(1)
					go r.watchSpaceIndexObjectType(
						workerCtx,
						ref.CloneVT(),
						id,
						workerGeneration,
						projectionCh,
						&wg,
					)
				}
				for id := range objectTypes {
					if _, ok := present[id]; !ok {
						delete(objectTypes, id)
					}
				}
			}

			if err := sendResourcesList(strm, currentList, objectTypes); err != nil {
				return err
			}
		case projection := <-projectionCh:
			worker, ok := workers[projection.id]
			if !ok || worker.generation != projection.generation {
				continue
			}
			changed := objectTypes[projection.id] != projection.objectType
			if projection.initial {
				if pendingInitial[projection.id] != projection.generation {
					continue
				}
				delete(pendingInitial, projection.id)
				if changed {
					objectTypes[projection.id] = projection.objectType
					initialProjectionChanged = true
				}
				if len(pendingInitial) != 0 || !initialProjectionChanged {
					continue
				}
				initialProjectionChanged = false
			} else {
				if !changed {
					continue
				}
				objectTypes[projection.id] = projection.objectType
				if len(pendingInitial) != 0 {
					initialProjectionChanged = true
					continue
				}
			}
			if err := sendResourcesList(strm, currentList, objectTypes); err != nil {
				return err
			}
		}
	}
}

// filterOutCdnSpace removes any SpaceSoListEntry whose block_store_id matches
// the well-known CDN Space. The anonymous CDN Space is reachable by ID via
// MountSharedObject(cdn.SpaceID()) and must never surface as an ordinary Space
// in enumerators like WatchResourcesList or GetTransferInventory.
func filterOutCdnSpace(list []*space.SpaceSoListEntry) []*space.SpaceSoListEntry {
	cdnID := cdn.SpaceID()
	out := list[:0]
	for _, ent := range list {
		if ent.GetEntry().GetRef().GetBlockStoreId() == cdnID {
			continue
		}
		out = append(out, ent)
	}
	return out
}

type resourcesListProjectionEvent struct {
	id         string
	generation uint64
	objectType string
	initial    bool
}

type resourcesListProjectionWorker struct {
	cancel     context.CancelFunc
	generation uint64
}

func (r *SessionResource) watchSpaceIndexObjectType(
	ctx context.Context,
	ref *sobject.SharedObjectRef,
	id string,
	generation uint64,
	events chan<- resourcesListProjectionEvent,
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	sendProjection := func(initial bool, objectType string) bool {
		select {
		case events <- resourcesListProjectionEvent{
			id:         id,
			generation: generation,
			objectType: objectType,
			initial:    initial,
		}:
			return true
		case <-ctx.Done():
			return false
		}
	}

	mountedSpace, mountedSpaceRef, err := space.ExMountSpaceSoBody(ctx, r.b, ref, true, nil)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			r.le.WithError(err).WithField("space-id", id).Warn("failed to project Space index ObjectType")
			sendProjection(true, "")
		}
		return
	}
	// A returnIfIdle miss is generic in the prompt batch; the blocking mount
	// below reports the durable type as a later live update.
	initialPending := true
	if mountedSpace == nil {
		if !sendProjection(true, "") {
			return
		}
		initialPending = false
		mountedSpace, mountedSpaceRef, err = space.ExMountSpaceSoBody(ctx, r.b, ref, false, nil)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				r.le.WithError(err).WithField("space-id", id).Warn("failed to project Space index ObjectType")
			}
			return
		}
	}
	defer mountedSpaceRef.Release()

	engine := mountedSpace.GetSharedObjectBody().GetWorldEngine()
	var previous string
	first := true
	for {
		objectType, seqno, err := readSpaceIndexObjectType(ctx, engine)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				r.le.WithError(err).WithField("space-id", id).Warn("failed to read Space index ObjectType")
				if initialPending {
					sendProjection(true, "")
				}
			}
			return
		}
		if first || objectType != previous {
			if !sendProjection(initialPending, objectType) {
				return
			}
			initialPending = false
			first = false
			previous = objectType
		}
		if _, err := engine.WaitSeqno(ctx, seqno+1); err != nil {
			if !errors.Is(err, context.Canceled) {
				r.le.WithError(err).WithField("space-id", id).Warn("failed to watch Space index ObjectType")
			}
			return
		}
	}
}

func readSpaceIndexObjectType(ctx context.Context, engine world.Engine) (string, uint64, error) {
	tx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		return "", 0, err
	}
	defer tx.Discard()

	seqno, err := tx.GetSeqno(ctx)
	if err != nil {
		return "", 0, err
	}
	objectType, err := space_world.LookupSpaceIndexObjectType(ctx, tx)
	if err != nil {
		return "", 0, err
	}
	return objectType, seqno, nil
}

func sendResourcesList(
	strm s4wave_session.SRPCSessionResourceService_WatchResourcesListStream,
	list []*space.SpaceSoListEntry,
	objectTypes map[string]string,
) error {
	for _, entry := range list {
		id := entry.GetEntry().GetRef().GetProviderResourceRef().GetId()
		entry.IndexObjectType = objectTypes[id]
	}
	return strm.Send(&s4wave_session.WatchResourcesListResponse{SpacesList: list})
}

// MountSharedObject mounts a shared object within the session by ID.
func (r *SessionResource) MountSharedObject(
	ctx context.Context,
	req *s4wave_session.MountSharedObjectRequest,
) (*s4wave_session.MountSharedObjectResponse, error) {
	sessionProviderResourceRef := r.session.GetSessionRef().GetProviderResourceRef()
	if err := sessionProviderResourceRef.Validate(); err != nil {
		return nil, err
	}

	soProviderResourceRef := sessionProviderResourceRef.CloneVT()
	soProviderResourceRef.Id = req.GetSharedObjectId()
	if err := soProviderResourceRef.Validate(); err != nil {
		return nil, err
	}

	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// CDN-backed SharedObjects are owned by the process-scoped CDN
	// registry and intentionally do not appear in the per-session SO
	// list. Route mounts for those ids directly to the anonymous
	// singleton so the normal SharedObject/SharedObjectBody pipeline
	// can dispatch on body_type downstream.
	if r.cdnLookup != nil {
		if cdnSO, meta := r.cdnLookup(req.GetSharedObjectId()); cdnSO != nil {
			return r.mountCdnSharedObject(resourceCtx, soProviderResourceRef, cdnSO, meta)
		}
	}

	// Find the shared object in the session list of shared objects.
	providerAcc := r.session.GetProviderAccount()
	soProvider, err := sobject.GetSharedObjectProviderAccountFeature(ctx, providerAcc)
	if err != nil {
		return nil, err
	}

	// TODO: pass released here?
	soListCtr, relSoListCtr, err := soProvider.AccessSharedObjectList(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer relSoListCtr()

	soListEntry, err := r.lookupSharedObjectListEntry(
		ctx,
		soProvider,
		soListCtr,
		req.GetSharedObjectId(),
	)
	if err != nil {
		return nil, err
	}
	if soListEntry == nil {
		return nil, sobject.ErrSharedObjectNotFound
	}

	soRef := &sobject.SharedObjectRef{
		ProviderResourceRef: soProviderResourceRef,
		BlockStoreId:        soListEntry.GetRef().GetBlockStoreId(),
	}
	if err := soRef.Validate(); err != nil {
		return nil, err
	}

	return r.addSharedObjectResource(ctx, resourceCtx, soRef, soListEntry.GetMeta())
}

// mountCdnSharedObject publishes the process-scoped CDN SharedObject as a
// SharedObjectResource on the caller's resource client. The CDN singleton
// is owned by the root CDN registry and lives for the lifetime of the
// process, so the release callback is a no-op.
func (r *SessionResource) mountCdnSharedObject(
	resourceCtx resource_server.ResourceClientContext,
	soProviderResourceRef *provider.ProviderResourceRef,
	cdnSO sobject.SharedObject,
	meta *sobject.SharedObjectMeta,
) (*s4wave_session.MountSharedObjectResponse, error) {
	bs := cdnSO.GetBlockStore()
	soRef := &sobject.SharedObjectRef{
		ProviderResourceRef: soProviderResourceRef,
		BlockStoreId:        bs.GetID(),
	}
	if err := soRef.Validate(); err != nil {
		return nil, err
	}

	soResource := resource_sobject.NewSharedObjectResourceWithHostPluginID(
		r.le,
		r.b,
		cdnSO,
		meta,
		soRef,
		r.session.GetPeerId().String(),
		r.hostPluginID,
	)
	id, err := resourceCtx.AddResource(soResource.GetMux(), func() {})
	if err != nil {
		return nil, err
	}

	return &s4wave_session.MountSharedObjectResponse{
		ResourceId:       id,
		SharedObjectMeta: meta,
		PeerId:           cdnSO.GetPeerID().String(),
		SharedObjectId:   cdnSO.GetSharedObjectID(),
		BlockStoreId:     bs.GetID(),
		HashType:         bs.GetHashType(),
	}, nil
}

// DeleteSpace deletes a space within the ProviderAccount.
func (r *SessionResource) DeleteSpace(ctx context.Context, req *s4wave_session.DeleteSpaceRequest) (*s4wave_session.DeleteSpaceResponse, error) {
	soID := req.GetSharedObjectId()
	if soID == "" {
		return nil, errors.New("shared_object_id is required")
	}

	providerAcc := r.session.GetProviderAccount()
	soFeature, err := sobject.GetSharedObjectProviderAccountFeature(ctx, providerAcc)
	if err != nil {
		return nil, err
	}

	if err := soFeature.DeleteSharedObject(ctx, soID); err != nil {
		return nil, err
	}

	return &s4wave_session.DeleteSpaceResponse{}, nil
}

// RenameSpace updates the display name metadata for a space.
func (r *SessionResource) RenameSpace(ctx context.Context, req *s4wave_session.RenameSpaceRequest) (*s4wave_session.RenameSpaceResponse, error) {
	soID := req.GetSharedObjectId()
	if soID == "" {
		return nil, errors.New("shared_object_id is required")
	}

	displayName := space.FixupSpaceName(req.GetDisplayName())
	soMeta, err := space.NewSharedObjectMeta(displayName)
	if err != nil {
		return nil, err
	}

	switch providerAcc := r.session.GetProviderAccount().(type) {
	case *provider_local.ProviderAccount:
		if err := providerAcc.UpdateSharedObjectMeta(ctx, soID, soMeta); err != nil {
			return nil, err
		}
	case *provider_spacewave.ProviderAccount:
		if _, err := providerAcc.UpdateSharedObjectMetadata(ctx, soID, &api.SpaceMetadataResponse{DisplayName: displayName}); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("rename space is not supported for this provider")
	}

	return &s4wave_session.RenameSpaceResponse{}, nil
}

// WatchLockState streams the current lock state and updates on changes.
func (r *SessionResource) WatchLockState(
	req *s4wave_session.WatchLockStateRequest,
	strm s4wave_session.SRPCSessionResourceService_WatchLockStateStream,
) error {
	return r.session.WatchLockState(strm.Context(), func(mode session.SessionLockMode, locked bool) {
		_ = strm.Send(&s4wave_session.WatchLockStateResponse{
			Mode:   mode,
			Locked: locked,
		})
	})
}

// SetLockMode changes the session lock mode.
func (r *SessionResource) SetLockMode(ctx context.Context, req *s4wave_session.SetLockModeRequest) (*s4wave_session.SetLockModeResponse, error) {
	if err := r.session.SetLockMode(ctx, req.GetMode(), req.GetPin()); err != nil {
		return nil, err
	}
	return &s4wave_session.SetLockModeResponse{}, nil
}

// SetDirectP2PEnabled updates the durable Cloud Session transport policy.
func (r *SessionResource) SetDirectP2PEnabled(
	ctx context.Context,
	req *s4wave_session.SetDirectP2PEnabledRequest,
) (*s4wave_session.SetDirectP2PEnabledResponse, error) {
	configurable, ok := r.session.(interface {
		SetDirectP2PEnabled(context.Context, bool) error
	})
	if !ok {
		return nil, errors.New("direct P2P policy is not supported for this provider")
	}
	if err := configurable.SetDirectP2PEnabled(ctx, req.GetEnabled()); err != nil {
		return nil, err
	}
	return &s4wave_session.SetDirectP2PEnabledResponse{}, nil
}

// UnlockSession unlocks a PIN-locked session.
func (r *SessionResource) UnlockSession(ctx context.Context, req *s4wave_session.UnlockSessionRequest) (*s4wave_session.UnlockSessionResponse, error) {
	if err := r.session.UnlockSession(ctx, req.GetPin()); err != nil {
		return nil, err
	}
	return &s4wave_session.UnlockSessionResponse{}, nil
}

// LockSession locks a running session.
func (r *SessionResource) LockSession(ctx context.Context, req *s4wave_session.LockSessionRequest) (*s4wave_session.LockSessionResponse, error) {
	if err := r.session.LockSession(ctx); err != nil {
		return nil, err
	}
	return &s4wave_session.LockSessionResponse{}, nil
}

// DeleteAccount deletes the entire account associated with this session.
// Cleans all session keys, removes GC edges, runs volume GC, deletes
// the volume backing store, and removes all sessions from the list.
func (r *SessionResource) DeleteAccount(ctx context.Context, req *s4wave_session.DeleteAccountRequest) (*s4wave_session.DeleteAccountResponse, error) {
	sessRef := r.session.GetSessionRef()
	provRef := sessRef.GetProviderResourceRef()
	providerID := provRef.GetProviderId()
	providerAccountID := provRef.GetProviderAccountId()

	// Look up the provider account to get the volume.
	providerAcc := r.session.GetProviderAccount()

	// Determine the volume, provider IRI, and object store ID based on provider type.
	var vol volume.Volume
	var providerIRI string
	var objectStoreID string
	switch acc := providerAcc.(type) {
	case *provider_local.ProviderAccount:
		vol = acc.GetVolume()
		providerIRI = provider_local.ProviderIRI(providerID)
		objectStoreID = provider_local.SessionObjectStoreID(providerID, providerAccountID)
	case *provider_spacewave.ProviderAccount:
		vol = acc.GetVolume()
		providerIRI = provider_spacewave.ProviderIRI(providerID)
		objectStoreID = provider_spacewave.SessionObjectStoreID(providerAccountID)
	default:
		return nil, errors.New("unsupported provider account type for delete")
	}

	// Look up all sessions for this provider account.
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, r.b, "", false, nil)
	if err != nil {
		return nil, errors.Wrap(err, "lookup session controller")
	}
	defer sessionCtrlRef.Release()

	allSessions, err := sessionCtrl.ListSessions(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "list sessions")
	}

	// Filter sessions belonging to this provider account.
	var accountSessions []*session.SessionListEntry
	for _, entry := range allSessions {
		ref := entry.GetSessionRef().GetProviderResourceRef()
		if ref.GetProviderId() == providerID && ref.GetProviderAccountId() == providerAccountID {
			accountSessions = append(accountSessions, entry)
		}
	}

	// Build ObjectStore handle for session keys.
	objStoreHandle, _, osRef, err := volume.ExBuildObjectStoreAPI(ctx, r.b, false, objectStoreID, vol.GetID(), nil)
	if err != nil {
		return nil, errors.Wrap(err, "build object store for session cleanup")
	}
	osReleased := false
	defer func() {
		if !osReleased {
			osRef.Release()
		}
	}()

	objStore := objStoreHandle.GetObjectStore()

	// Collect linked-cloud account IDs before deleting keys.
	var linkedCloudAccountIDs []string
	if providerID == "local" {
		rtx, err := objStore.NewTransaction(ctx, false)
		if err == nil {
			for _, entry := range accountSessions {
				sid := entry.GetSessionRef().GetProviderResourceRef().GetId()
				key := provider_local.LinkedCloudKey(sid)
				data, found, gerr := rtx.Get(ctx, key)
				if gerr == nil && found && len(data) > 0 {
					linkedCloudAccountIDs = append(linkedCloudAccountIDs, string(data))
				}
			}
			rtx.Discard()
		}
	}

	// Clean all session keys in the ObjectStore.
	for _, entry := range accountSessions {
		sid := entry.GetSessionRef().GetProviderResourceRef().GetId()
		tx, err := objStore.NewTransaction(ctx, true)
		if err != nil {
			return nil, errors.Wrap(err, "open transaction for session key cleanup")
		}
		prefix := []byte(sid + "/")
		if err := tx.ScanPrefixKeys(ctx, prefix, func(key []byte) error {
			return tx.Delete(ctx, key)
		}); err != nil {
			tx.Discard()
			return nil, errors.Wrap(err, "scan and delete session keys")
		}
		if err := tx.Commit(ctx); err != nil {
			tx.Discard()
			return nil, errors.Wrap(err, "commit session key cleanup")
		}
	}

	// Remove GC root edge and run collection.
	if kvVol, ok := vol.(kvtx_volume.KvtxVolume); ok {
		if rg := kvVol.GetRefGraph(); rg != nil {
			gcOps := block_gc.NewGCStoreOps(vol, rg)
			if err := gcOps.RemoveGCRef(ctx, block_gc.NodeGCRoot, providerIRI); err != nil {
				r.le.WithError(err).Warn("failed to remove gc root ref for deleted account")
			}

			collector := block_gc.NewCollector(rg, vol, nil)
			if _, err := collector.Collect(ctx); err != nil {
				r.le.WithError(err).Warn("gc collect after account delete failed")
			}
		}
	}

	// Release the ObjectStore handle before session teardown.
	osRef.Release()
	osReleased = true

	// Remove all session list entries for this account first. This
	// triggers background goroutine shutdown and releases their IDB
	// connections, which is required before vol.Delete() can proceed
	// (IndexedDB deleteDatabase blocks while connections remain open).
	for _, entry := range accountSessions {
		if err := sessionCtrl.DeleteSession(ctx, entry.GetSessionRef()); err != nil {
			r.le.WithError(err).Warn("failed to delete session from list")
		}
	}

	// Delete the volume backing store (close + remove file/database).
	if err := vol.Delete(); err != nil {
		r.le.WithError(err).Warn("failed to delete volume backing store")
	}

	// Best-effort unlink: clean cloud-side linked-local keys.
	for _, cloudAccountID := range linkedCloudAccountIDs {
		// Find the cloud session matching this cloud account ID.
		for _, entry := range allSessions {
			ref := entry.GetSessionRef().GetProviderResourceRef()
			if ref.GetProviderId() != "spacewave" || ref.GetProviderAccountId() != cloudAccountID {
				continue
			}
			// Look up the spacewave provider via the bus.
			swProv, swProvRef, aerr := provider.ExLookupProvider(ctx, r.b, "spacewave", false, nil)
			if aerr != nil {
				r.le.WithError(aerr).Warn("failed to lookup spacewave provider for unlink")
				break
			}
			swAcc, relSw, aerr := swProv.AccessProviderAccount(ctx, cloudAccountID, nil)
			if aerr != nil {
				swProvRef.Release()
				r.le.WithError(aerr).Warn("failed to access cloud account for unlink")
				break
			}
			if swPA, ok := swAcc.(*provider_spacewave.ProviderAccount); ok {
				sid := ref.GetId()
				if uerr := swPA.DeleteLinkedLocalSession(ctx, sid); uerr != nil {
					r.le.WithError(uerr).Warn("failed to unlink cloud-side linked-local key")
				}
			}
			relSw()
			swProvRef.Release()
			break
		}
	}

	return &s4wave_session.DeleteAccountResponse{}, nil
}

// mountInviteHost mounts a space shared object by ID and returns the
// InviteHost interface. Caller must defer releaseFn.
func (r *SessionResource) mountInviteHost(
	ctx context.Context,
	spaceID string,
) (sobject.InviteHost, func(), error) {
	providerAcc := r.session.GetProviderAccount()
	soFeature, err := sobject.GetSharedObjectProviderAccountFeature(ctx, providerAcc)
	if err != nil {
		return nil, nil, err
	}

	soListCtr, relSoListCtr, err := soFeature.AccessSharedObjectList(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer relSoListCtr()

	soListEntry, err := r.lookupSharedObjectListEntry(ctx, soFeature, soListCtr, spaceID)
	if err != nil {
		return nil, nil, err
	}
	var blockStoreID string
	if soListEntry != nil {
		blockStoreID = soListEntry.GetRef().GetBlockStoreId()
	} else if _, ok := providerAcc.(*provider_spacewave.ProviderAccount); ok {
		blockStoreID = provider_spacewave.SobjectBlockStoreID(spaceID)
	} else {
		return nil, nil, errors.Wrap(
			sobject.ErrSharedObjectNotFound,
			"lookup shared object list entry",
		)
	}
	sessRef := r.session.GetSessionRef().GetProviderResourceRef()
	soRef := &sobject.SharedObjectRef{
		ProviderResourceRef: &provider.ProviderResourceRef{
			ProviderId:        sessRef.GetProviderId(),
			ProviderAccountId: sessRef.GetProviderAccountId(),
			Id:                spaceID,
		},
		BlockStoreId: blockStoreID,
	}

	mountedSo, mountedSoRef, err := sobject.ExMountSharedObject(ctx, r.session.GetBus(), soRef, false, nil)
	if err != nil {
		return nil, nil, errors.Wrap(err, "mount shared object")
	}

	ih, ok := mountedSo.(sobject.InviteHost)
	if !ok {
		mountedSoRef.Release()
		return nil, nil, errors.New("shared object does not support invites")
	}

	return ih, mountedSoRef.Release, nil
}

// lookupSharedObjectListEntry resolves a shared object list entry and forces a
// fresh cloud snapshot before returning not found.
func (r *SessionResource) lookupSharedObjectListEntry(
	ctx context.Context,
	soFeature sobject.SharedObjectProvider,
	soListCtr ccontainer.Watchable[*sobject.SharedObjectList],
	sharedObjectID string,
) (*sobject.SharedObjectListEntry, error) {
	soList, err := soListCtr.WaitValue(ctx, nil)
	if err != nil {
		return nil, err
	}
	soIdx := slices.IndexFunc(soList.GetSharedObjects(), func(so *sobject.SharedObjectListEntry) bool {
		return so.GetRef().GetProviderResourceRef().GetId() == sharedObjectID
	})
	if soIdx != -1 {
		return soList.GetSharedObjects()[soIdx], nil
	}

	if err := soFeature.RefreshSharedObjectList(ctx); err != nil {
		return nil, err
	}

	soList = soListCtr.GetValue()
	soIdx = slices.IndexFunc(soList.GetSharedObjects(), func(so *sobject.SharedObjectListEntry) bool {
		return so.GetRef().GetProviderResourceRef().GetId() == sharedObjectID
	})
	if soIdx == -1 {
		return nil, nil
	}
	return soList.GetSharedObjects()[soIdx], nil
}

// CreateSpaceInvite creates an invite for a space shared object.
func (r *SessionResource) CreateSpaceInvite(
	ctx context.Context,
	req *s4wave_session.CreateSpaceInviteRequest,
) (*s4wave_session.CreateSpaceInviteResponse, error) {
	spaceID := req.GetSpaceId()
	if spaceID == "" {
		return nil, errors.New("space_id is required")
	}

	ih, rel, err := r.mountInviteHost(ctx, spaceID)
	if err != nil {
		return nil, errors.Wrap(err, "mount invite host")
	}
	defer rel()

	msg, err := ih.CreateSOInviteOp(
		ctx,
		ih.GetPrivKey(),
		req.GetRole(),
		ih.GetProviderID(),
		req.GetTargetPeerId(),
		req.GetMaxUses(),
		req.GetExpiresAt(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "create invite")
	}

	resp := &s4wave_session.CreateSpaceInviteResponse{InviteMessage: msg}

	// For spacewave sessions, register a short code with the cloud.
	if swAcc, ok := r.session.GetProviderAccount().(*provider_spacewave.ProviderAccount); ok {
		var expiresAt int64
		if exp := req.GetExpiresAt(); exp != nil {
			expiresAt = exp.GetSeconds() * 1000
		}

		tokenHashHex := hex.EncodeToString(
			sobject_invite.HashInviteToken(msg.GetToken()),
		)
		if tokenHashHex != "" {
			if err := swAcc.GetSessionClient().RegisterInviteBeacon(
				ctx,
				spaceID,
				msg.GetInviteId(),
				tokenHashHex,
				expiresAt,
			); err != nil {
				r.le.WithError(err).Warn("failed to register invite beacon")
			}
		}

		code := generateShortCode()
		msgData, err := msg.MarshalVT()
		if err == nil {
			if err := swAcc.GetSessionClient().RegisterInviteCode(ctx, spaceID, &api.RegisterInviteCodeRequest{
				Code:          code,
				InviteId:      msg.GetInviteId(),
				InviteMessage: base64.StdEncoding.EncodeToString(msgData),
				ExpiresAt:     expiresAt,
			}); err != nil {
				r.le.WithError(err).Warn("failed to register invite short code")
			} else {
				resp.ShortCode = code
			}
		}
	}

	return resp, nil
}

// generateShortCode returns a random 8-character alphanumeric code.
func generateShortCode() string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	for i := range buf {
		buf[i] = alphabet[int(buf[i])%len(alphabet)]
	}
	return string(buf)
}

// ListSpaceInvites lists invites on a space shared object.
func (r *SessionResource) ListSpaceInvites(
	ctx context.Context,
	req *s4wave_session.ListSpaceInvitesRequest,
) (*s4wave_session.ListSpaceInvitesResponse, error) {
	spaceID := req.GetSpaceId()
	if spaceID == "" {
		return nil, errors.New("space_id is required")
	}

	ih, rel, err := r.mountInviteHost(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	defer rel()

	state, err := ih.GetSOHost().GetHostState(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get shared object state")
	}

	return &s4wave_session.ListSpaceInvitesResponse{Invites: state.GetInvites()}, nil
}

// ListSpaceParticipants lists participants on a space shared object.
func (r *SessionResource) ListSpaceParticipants(
	ctx context.Context,
	req *s4wave_session.ListSpaceParticipantsRequest,
) (*s4wave_session.ListSpaceParticipantsResponse, error) {
	spaceID := req.GetSpaceId()
	if spaceID == "" {
		return nil, errors.New("space_id is required")
	}

	ih, rel, err := r.mountInviteHost(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	defer rel()

	state, err := ih.GetSOHost().GetHostState(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get shared object state")
	}

	return &s4wave_session.ListSpaceParticipantsResponse{
		Participants: state.GetConfig().GetParticipants(),
	}, nil
}

// RemoveSpaceParticipant removes a participant from a space shared object by peer ID.
func (r *SessionResource) RemoveSpaceParticipant(
	ctx context.Context,
	req *s4wave_session.RemoveSpaceParticipantRequest,
) (*s4wave_session.RemoveSpaceParticipantResponse, error) {
	spaceID := req.GetSpaceId()
	if spaceID == "" {
		return nil, errors.New("space_id is required")
	}
	peerID := req.GetPeerId()
	if peerID == "" {
		return nil, errors.New("peer_id is required")
	}

	ih, rel, err := r.mountInviteHost(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	defer rel()

	removed, err := sobject.RemoveSOParticipant(ctx, ih.GetSOHost(), peerID, ih.GetPrivKey(), nil)
	if err != nil {
		return nil, errors.Wrap(err, "remove participant")
	}

	return &s4wave_session.RemoveSpaceParticipantResponse{Removed: removed}, nil
}

// RevokeSpaceInvite revokes an invite on a space shared object.
func (r *SessionResource) RevokeSpaceInvite(
	ctx context.Context,
	req *s4wave_session.RevokeSpaceInviteRequest,
) (*s4wave_session.RevokeSpaceInviteResponse, error) {
	spaceID := req.GetSpaceId()
	if spaceID == "" {
		return nil, errors.New("space_id is required")
	}
	inviteID := req.GetInviteId()
	if inviteID == "" {
		return nil, errors.New("invite_id is required")
	}

	ih, rel, err := r.mountInviteHost(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	defer rel()

	if err := ih.RevokeInvite(ctx, ih.GetPrivKey(), inviteID); err != nil {
		return nil, errors.Wrap(err, "revoke invite")
	}

	return &s4wave_session.RevokeSpaceInviteResponse{}, nil
}

// JoinSpaceViaInvite joins a space using an out-of-band invite message.
func (r *SessionResource) JoinSpaceViaInvite(
	ctx context.Context,
	req *s4wave_session.JoinSpaceViaInviteRequest,
) (*s4wave_session.JoinSpaceViaInviteResponse, error) {
	inviteMsg := req.GetInviteMessage()
	if inviteMsg == nil {
		return nil, errors.New("invite_message is required")
	}

	sessionKey := r.session.GetPrivKey()
	if sessionKey == nil {
		return nil, errors.New("session is locked")
	}

	switch acc := r.session.GetProviderAccount().(type) {
	case *provider_local.ProviderAccount:
		result, err := acc.JoinViaInvite(ctx, sessionKey, inviteMsg, "")
		if err != nil {
			if errors.Is(err, provider_local.ErrDirectInviteOwnerMustBeOnline) {
				return &s4wave_session.JoinSpaceViaInviteResponse{
					SharedObjectId: inviteMsg.GetSharedObjectId(),
					Result:         s4wave_session.JoinSpaceViaInviteResult_JoinSpaceViaInviteResult_OWNER_MUST_BE_ONLINE,
				}, nil
			}
			return nil, err
		}
		return &s4wave_session.JoinSpaceViaInviteResponse{
			SharedObjectId: result.SharedObjectID,
			Result:         s4wave_session.JoinSpaceViaInviteResult_JoinSpaceViaInviteResult_ACCEPTED,
		}, nil
	case *provider_spacewave.ProviderAccount:
		const inviteAcceptFastPathTimeout = time.Second

		joinResp, err := sobject_invite.BuildJoinResponse(inviteMsg.GetInviteId(), sessionKey)
		if err != nil {
			return nil, errors.Wrap(err, "build cloud join response")
		}
		var targetedEnvelope *api.TargetedInvitationEnvelope
		if data := req.GetTargetedInvitationEnvelope(); len(data) > 0 {
			targetedEnvelope = &api.TargetedInvitationEnvelope{}
			if err := targetedEnvelope.UnmarshalVT(data); err != nil {
				return nil, errors.Wrap(err, "unmarshal targeted invitation envelope")
			}
		}
		cli := acc.GetSessionClient()
		if cli == nil {
			return nil, errors.New("session client not ready")
		}
		acc.TrackMailboxRequest(
			inviteMsg.GetSharedObjectId(),
			inviteMsg.GetInviteId(),
			r.session.GetPeerId().String(),
			"pending",
		)
		submitResp, err := cli.SubmitMailboxEntry(ctx, inviteMsg.GetSharedObjectId(), &api.SubmitMailboxEntryRequest{
			InviteId:         inviteMsg.GetInviteId(),
			Token:            inviteMsg.GetToken(),
			JoinResponse:     joinResp,
			TargetedEnvelope: targetedEnvelope,
		})
		if err != nil {
			return nil, err
		}
		status := submitResp.GetStatus()
		if status != "" {
			acc.TrackMailboxRequest(
				inviteMsg.GetSharedObjectId(),
				inviteMsg.GetInviteId(),
				r.session.GetPeerId().String(),
				status,
			)
		}
		if status == "accepted" {
			return acceptedCloudInviteJoinResponse(ctx, acc, inviteMsg.GetSharedObjectId())
		}
		waitCtx, waitCancel := context.WithTimeout(ctx, inviteAcceptFastPathTimeout)
		defer waitCancel()
		status, err = acc.WaitMailboxRequestDecision(
			waitCtx,
			inviteMsg.GetSharedObjectId(),
			inviteMsg.GetInviteId(),
			r.session.GetPeerId().String(),
		)
		if err == nil {
			if status == "accepted" {
				return acceptedCloudInviteJoinResponse(ctx, acc, inviteMsg.GetSharedObjectId())
			}
			if status == "rejected" {
				return &s4wave_session.JoinSpaceViaInviteResponse{
					SharedObjectId: inviteMsg.GetSharedObjectId(),
					Result:         s4wave_session.JoinSpaceViaInviteResult_JoinSpaceViaInviteResult_REJECTED,
				}, nil
			}
		} else if !errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return &s4wave_session.JoinSpaceViaInviteResponse{
			SharedObjectId: inviteMsg.GetSharedObjectId(),
			Result:         s4wave_session.JoinSpaceViaInviteResult_JoinSpaceViaInviteResult_PENDING_OWNER_APPROVAL,
		}, nil
	default:
		return nil, errors.New("unsupported provider type for invite join")
	}
}

func acceptedCloudInviteJoinResponse(
	ctx context.Context,
	acc acceptedCloudInviteAccount,
	soID string,
) (*s4wave_session.JoinSpaceViaInviteResponse, error) {
	if err := acc.RefreshSharedObjectList(ctx); err != nil {
		return nil, errors.Wrap(err, "refresh shared object list after accepted invite")
	}
	return &s4wave_session.JoinSpaceViaInviteResponse{
		SharedObjectId: soID,
		Result:         s4wave_session.JoinSpaceViaInviteResult_JoinSpaceViaInviteResult_ACCEPTED,
	}, nil
}

// _ is a type assertion
var _ s4wave_session.SRPCSessionResourceServiceServer = ((*SessionResource)(nil))
