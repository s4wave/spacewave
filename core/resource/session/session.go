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
	resource_sobject "github.com/s4wave/spacewave/core/resource/sobject"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_invite "github.com/s4wave/spacewave/core/sobject/invite"
	"github.com/s4wave/spacewave/core/space"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/volume"
	kvtx_volume "github.com/s4wave/spacewave/db/volume/common/kvtx"
	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/util/confparse"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	s4wave_status "github.com/s4wave/spacewave/sdk/status"
	"github.com/sirupsen/logrus"
)

// SessionResource wraps a core session for resource access.
//
// The lifecycle context ctx scopes any background work owned by this
// resource. It is canceled by Close when the mount is released.
type SessionResource struct {
	le          *logrus.Entry
	b           bus.Bus
	mux         srpc.Invoker
	session     session.Session
	transferMgr transferManager

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

// NewSessionResource creates a new SessionResource.
func NewSessionResource(le *logrus.Entry, b bus.Bus, sess session.Session) *SessionResource {
	ctx, ctxCancel := context.WithCancel(context.Background())
	sessResource := &SessionResource{
		le:        le,
		b:         b,
		session:   sess,
		ctx:       ctx,
		ctxCancel: ctxCancel,
	}

	statusRes := NewStatusResource(b)
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
	resp.CryptoInfo = r.buildCryptoInfo(ctx)
	return resp, nil
}

// buildCryptoInfo extracts crypto identity and storage stats from the session.
func (r *SessionResource) buildCryptoInfo(ctx context.Context) *s4wave_session.SessionCryptoInfo {
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

	// Populate storage stats if the provider account supports it.
	providerAcc := r.session.GetProviderAccount()
	if ssp, ok := providerAcc.(provider.StorageStatsProvider); ok {
		stats, err := ssp.GetStorageStats(ctx)
		if err == nil && stats != nil {
			info.TotalStorageBytes = stats.GetTotalBytes()
		}
	}

	// Populate space count from the shared object list.
	soProvider, err := sobject.GetSharedObjectProviderAccountFeature(ctx, providerAcc)
	if err == nil {
		soListWatchable, relSoList, err := soProvider.AccessSharedObjectList(ctx, nil)
		if err == nil {
			soList := soListWatchable.GetValue()
			if soList != nil {
				info.SpaceCount = uint32(len(soList.GetSharedObjects()))
			}
			relSoList()
		}
	}

	return info
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
	// Initialize the space world immediately so later readers and writers do not
	// block waiting for the first owner mount to seed the head state.
	_, spaceBodyRef, err := space.ExMountSpaceSoBody(ctx, r.b, soRef, false, nil)
	if err != nil {
		return nil, err
	}
	defer spaceBodyRef.Release()

	return &s4wave_session.CreateSpaceResponse{
		SharedObjectRef:  soRef,
		SharedObjectMeta: soMeta,
	}, nil
}

// WatchResourcesList returns the list of resources the session has access to.
func (r *SessionResource) WatchResourcesList(
	req *s4wave_session.WatchResourcesListRequest,
	strm s4wave_session.SRPCSessionResourceService_WatchResourcesListStream,
) error {
	// get the shared object list container
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

	// watch the shared object list, find the matching value, write the response.
	return ccontainer.WatchChanges(
		ctx,
		nil,
		soListWatchable,
		func(soList *sobject.SharedObjectList) error {
			// wait for list to be present
			if soList == nil {
				return nil
			}

			// match spaces
			list, err := space.FilterSharedObjectList(soList.GetSharedObjects(), func(ent *sobject.SharedObjectListEntry, err error) error {
				return nil
			})
			if err != nil {
				return err
			}

			// Drop the well-known CDN Space from the resource list; it is
			// mounted by ID via MountSharedObject(cdn.SpaceID()) and must not
			// appear as an ordinary Space in the UI.
			list = filterOutCdnSpace(list)

			return strm.Send(&s4wave_session.WatchResourcesListResponse{SpacesList: list})
		},
		nil,
	)
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
		providerAcc,
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

	// TODO: pass released here?
	mountedSo, mountedSoRef, err := sobject.ExMountSharedObject(ctx, r.session.GetBus(), soRef, false, nil)
	if err != nil {
		return nil, err
	}

	soResource := resource_sobject.NewSharedObjectResource(r.le, r.b, mountedSo, soListEntry.GetMeta(), soRef)
	id, err := resourceCtx.AddResource(soResource.GetMux(), mountedSoRef.Release)
	if err != nil {
		mountedSoRef.Release()
		return nil, err
	}

	return &s4wave_session.MountSharedObjectResponse{
		ResourceId:       id,
		SharedObjectMeta: soListEntry.GetMeta(),
		PeerId:           mountedSo.GetPeerID().String(),
		SharedObjectId:   mountedSo.GetSharedObjectID(),
		BlockStoreId:     mountedSo.GetBlockStore().GetID(),
		HashType:         mountedSo.GetBlockStore().GetHashType(),
	}, nil
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

	soResource := resource_sobject.NewSharedObjectResource(r.le, r.b, cdnSO, meta, soRef)
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

	soListEntry, err := r.lookupSharedObjectListEntry(
		ctx,
		providerAcc,
		soListCtr,
		spaceID,
	)
	if err != nil {
		return nil, nil, err
	}
	if soListEntry == nil {
		return nil, nil, sobject.ErrSharedObjectNotFound
	}
	sessRef := r.session.GetSessionRef().GetProviderResourceRef()
	soRef := &sobject.SharedObjectRef{
		ProviderResourceRef: &provider.ProviderResourceRef{
			ProviderId:        sessRef.GetProviderId(),
			ProviderAccountId: sessRef.GetProviderAccountId(),
			Id:                spaceID,
		},
		BlockStoreId: soListEntry.GetRef().GetBlockStoreId(),
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
	providerAcc provider.ProviderAccount,
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

	swAcc, ok := providerAcc.(*provider_spacewave.ProviderAccount)
	if !ok {
		return nil, nil
	}
	if err := swAcc.RefreshSharedObjectList(ctx); err != nil {
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
		return nil, err
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
			InviteId:     inviteMsg.GetInviteId(),
			Token:        inviteMsg.GetToken(),
			JoinResponse: joinResp,
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
			return &s4wave_session.JoinSpaceViaInviteResponse{
				SharedObjectId: inviteMsg.GetSharedObjectId(),
				Result:         s4wave_session.JoinSpaceViaInviteResult_JoinSpaceViaInviteResult_ACCEPTED,
			}, nil
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
				return &s4wave_session.JoinSpaceViaInviteResponse{
					SharedObjectId: inviteMsg.GetSharedObjectId(),
					Result:         s4wave_session.JoinSpaceViaInviteResult_JoinSpaceViaInviteResult_ACCEPTED,
				}, nil
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

// _ is a type assertion
var _ s4wave_session.SRPCSessionResourceServiceServer = ((*SessionResource)(nil))
