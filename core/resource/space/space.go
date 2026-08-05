package resource_space

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	plugin_space "github.com/s4wave/spacewave/core/plugin/space"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	"github.com/s4wave/spacewave/core/resource/space/hostplugin"
	"github.com/s4wave/spacewave/core/resource/space/sharingstate"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	s4wave_wizard "github.com/s4wave/spacewave/sdk/world/wizard"
	"github.com/sirupsen/logrus"
)

// SpaceResource wraps a Space for resource access.
type SpaceResource struct {
	le            *logrus.Entry
	b             bus.Bus
	mux           srpc.Invoker
	space         space.SpaceSharedObjectBody
	sessionPeerID string
	hostPluginID  string
}

// NewSpaceResource creates a new SpaceResource.
func NewSpaceResource(le *logrus.Entry, b bus.Bus, sp space.SpaceSharedObjectBody) *SpaceResource {
	return NewSpaceResourceWithSessionPeerID(le, b, sp, "")
}

// NewSpaceResourceWithSessionPeerID creates a new SpaceResource with the
// mounted local session peer ID.
func NewSpaceResourceWithSessionPeerID(
	le *logrus.Entry,
	b bus.Bus,
	sp space.SpaceSharedObjectBody,
	sessionPeerID string,
) *SpaceResource {
	return NewSpaceResourceWithSessionPeerIDAndHostPluginID(le, b, sp, sessionPeerID, "")
}

// NewSpaceResourceWithSessionPeerIDAndHostPluginID creates a SpaceResource
// with the mounted local session peer ID and host plugin ID.
func NewSpaceResourceWithSessionPeerIDAndHostPluginID(
	le *logrus.Entry,
	b bus.Bus,
	sp space.SpaceSharedObjectBody,
	sessionPeerID string,
	hostPluginID string,
) *SpaceResource {
	spaceResource := &SpaceResource{
		le:            le,
		b:             b,
		space:         sp,
		sessionPeerID: sessionPeerID,
		hostPluginID:  hostPluginID,
	}
	spaceResource.mux = resource_server.NewResourceMux(
		func(mux srpc.Mux) error {
			return s4wave_space.SRPCRegisterSpaceResourceService(mux, spaceResource)
		},
		func(mux srpc.Mux) error {
			wizardResource := s4wave_wizard.NewWizardRegistryResource()
			return s4wave_wizard.SRPCRegisterObjectWizardRegistryResourceService(mux, wizardResource)
		},
	)
	return spaceResource
}

func (r *SpaceResource) resolveHostPluginID(ctx context.Context) string {
	return hostplugin.Resolve(ctx, r.hostPluginID)
}

// GetMux returns the rpc mux.
func (r *SpaceResource) GetMux() srpc.Invoker {
	return r.mux
}

// GetWorldEngine returns the Space world engine.
func (r *SpaceResource) GetWorldEngine() world.Engine {
	return r.space.GetWorldEngine()
}

// GetWorldEngineID returns the Space world engine id.
func (r *SpaceResource) GetWorldEngineID() string {
	return r.space.GetWorldEngineID()
}

// GetWorldEngineBucketID returns the Space world engine bucket id.
func (r *SpaceResource) GetWorldEngineBucketID() string {
	return r.space.GetWorldEngineBucketID()
}

// WatchSpaceState watches the SpaceState for the component.
func (r *SpaceResource) WatchSpaceState(
	req *s4wave_space.WatchSpaceStateRequest,
	strm s4wave_space.SRPCSpaceResourceService_WatchSpaceStateStream,
) error {
	// Initialize the world snapshot watch.
	ctx, worldEng := strm.Context(), r.space.GetWorldEngine()

	// Read and publish the current world contents.
	var prevWorldSeqno uint64
	for {
		r.le.Debugf("checking world state: seqno(%v)", prevWorldSeqno+1)

		// Build one SpaceState snapshot from a read transaction.
		var state *s4wave_space.SpaceState
		if err := func() error {
			wtx, err := worldEng.NewTransaction(ctx, false)
			if err != nil {
				return err
			}
			defer wtx.Discard()

			prevWorldSeqno, err = wtx.GetSeqno(ctx)
			if err != nil {
				return err
			}

			// Start a ready SpaceState response.
			state = &s4wave_space.SpaceState{Ready: true}

			// Build the world object list.
			state.WorldContents, err = space_world.BuildWorldContents(ctx, wtx)
			if err != nil {
				return err
			}

			// Load SpaceSettings when present.
			state.Settings, _, err = space_world.LookupSpaceSettings(ctx, wtx)
			if err != nil {
				return err
			}

			// Attach shared-object transform information.
			state.TransformInfo = r.buildTransformInfo(ctx)

			// Publish the snapshot.
			return strm.Send(state)
		}(); err != nil {
			return err
		}

		// Wait for the next world sequence.
		if _, err := worldEng.WaitSeqno(ctx, prevWorldSeqno+1); err != nil {
			return err
		}
	}
}

// WatchSpaceSharingState watches the sharing snapshot for the space.
//
// All change sources (SO state, mailbox metadata) are folded into one local
// broadcast so the watch loop reads every input snapshot under the same
// HoldLock that obtains the wait channel. This eliminates the missed-wakeup
// race that the previous dual-channel select had to defend against with
// per-source buffered signals, and coalesces near-simultaneous source
// changes into a single emission instead of one emission per source.
func (r *SpaceResource) WatchSpaceSharingState(
	req *s4wave_space.WatchSpaceSharingStateRequest,
	strm s4wave_space.SRPCSpaceResourceService_WatchSpaceSharingStateStream,
) error {
	ctx := strm.Context()
	inviteHost, ok := r.space.GetSharedObject().(sobject.InviteHost)
	if !ok {
		return nil
	}

	soStateCtr, relSoStateCtr, err := inviteHost.GetSOHost().GetSOStateCtr(ctx, nil)
	if err != nil {
		return err
	}
	defer relSoStateCtr()

	soState, err := soStateCtr.WaitValue(ctx, nil)
	if err != nil {
		return err
	}

	swAcc, releaseMailboxAcc, err := r.accessMailboxProviderAccount(ctx)
	if err != nil {
		return err
	}
	if releaseMailboxAcc != nil {
		defer releaseMailboxAcc()
	}
	soID := r.space.GetSharedObjectRef().GetProviderResourceRef().GetId()
	if swAcc != nil {
		if _, err := swAcc.GetPendingMailboxEntriesCached(ctx, soID); err != nil {
			r.le.WithError(err).Warn("failed to prime mailbox cache")
		}
	}
	presentationState := loadSharingParticipantPresentationState(ctx, r.le, swAcc, soID)

	var mailboxEntries []*sharingstate.MailboxEntry
	if swAcc != nil {
		snapshot, _ := swAcc.GetPendingMailboxEntriesSnapshot(soID)
		mailboxEntries = sharingMailboxEntriesFromProto(snapshot)
	}
	state := sharingstate.NewState(soState, mailboxEntries, presentationState)

	bridgeCtx, cancelBridges := context.WithCancel(ctx)
	defer cancelBridges()
	go state.BridgeSOState(bridgeCtx, soStateCtr)
	if swAcc != nil {
		go bridgeSharingMailbox(bridgeCtx, state, swAcc, soID)
	}

	peerID := r.space.GetSharedObject().GetPeerID().String()
	return state.RunWatchLoop(ctx, peerID, func(state *sharingstate.SharingState) error {
		return strm.Send(sharingStateToProto(state))
	})
}

// buildTransformInfo extracts redacted transform info from the shared object state.
func (r *SpaceResource) buildTransformInfo(ctx context.Context) *s4wave_space.TransformInfo {
	so := r.space.GetSharedObject()
	snap, err := so.GetSharedObjectState(ctx)
	if err != nil {
		return nil
	}
	info, err := snap.GetTransformInfo(ctx)
	if err != nil || info == nil {
		return nil
	}
	return r.transformInfoToProto(info)
}

// transformInfoToProto converts a sobject.TransformInfo to the proto message.
func (r *SpaceResource) transformInfoToProto(info *sobject.TransformInfo) *s4wave_space.TransformInfo {
	return &s4wave_space.TransformInfo{
		Steps:      info.Steps,
		GrantCount: info.GrantCount,
	}
}

// accessMailboxProviderAccount returns the provider account backing mailbox cache state.
func (r *SpaceResource) accessMailboxProviderAccount(
	ctx context.Context,
) (*provider_spacewave.ProviderAccount, func(), error) {
	ref := r.space.GetSharedObjectRef().GetProviderResourceRef()
	if ref.GetProviderId() != "spacewave" {
		return nil, nil, nil
	}

	provAcc, relProvAcc, err := provider.ExAccessProviderAccount(
		ctx,
		r.b,
		ref.GetProviderId(),
		ref.GetProviderAccountId(),
		false,
		nil,
	)
	if err != nil {
		return nil, nil, err
	}

	swAcc, ok := provAcc.(*provider_spacewave.ProviderAccount)
	if !ok {
		relProvAcc.Release()
		return nil, nil, nil
	}
	return swAcc, relProvAcc.Release, nil
}

// AccessWorld accesses the World associated with the space.
func (r *SpaceResource) AccessWorld(
	ctx context.Context,
	req *s4wave_space.AccessWorldRequest,
) (*s4wave_space.AccessWorldResponse, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	sessionPeerID := peer.ID("")
	if r.sessionPeerID != "" {
		sessionPeerID, err = peer.IDB58Decode(r.sessionPeerID)
		if err != nil {
			return nil, err
		}
	}

	lookupOp := space_world_optypes.BuildSpaceLookupOp(r.b, r.le, r.space.GetWorldEngineID())
	engineInfo := &s4wave_world.EngineInfo{
		EngineId: r.space.GetWorldEngineID(),
		BucketId: r.space.GetWorldEngineBucketID(),
	}
	worldResource := resource_world.NewEngineResource(
		r.le,
		r.b,
		r.space.GetWorldEngine(),
		lookupOp,
		engineInfo,
		resource_world.WithSessionPeerID(sessionPeerID),
	)
	id, err := resourceCtx.AddResource(worldResource.GetMux(), func() {})
	if err != nil {
		return nil, err
	}

	return &s4wave_space.AccessWorldResponse{ResourceId: id}, nil
}

// MountSpaceContents activates plugins for the space and returns a sub-resource
// for monitoring plugin status.
func (r *SpaceResource) MountSpaceContents(
	ctx context.Context,
	req *s4wave_space.MountSpaceContentsRequest,
) (*s4wave_space.MountSpaceContentsResponse, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		r.le.WithError(err).Info("failed to mount space contents: missing client context")
		return nil, err
	}

	ref := r.space.GetSharedObjectRef()
	spaceID := space.SpaceEngineId(ref)
	engineID := r.space.GetWorldEngineID()

	// Create the contents sub-resource.
	contentsResource := NewSpaceContentsResource(r.le, r.b, r.space.GetWorldEngine(), spaceID, engineID)

	// Resolve the host plugin id from the request context. The Space resource
	// runs inside a plugin host; cloud block store forwarding registers under
	// that host's plugin service prefix.
	hostPluginID := r.resolveHostPluginID(ctx)

	worldBucketID := r.space.GetWorldEngineBucketID()
	if worldBucketID != "" && hostPluginID == "" {
		// Without a host plugin id we cannot route the forwarded block store.
		// Skip forwarding rather than registering under a guessed prefix.
		worldBucketID = ""
	}

	// Start the plugin/space controller through the contents resource. The
	// controller may need to acquire document-owned plugin-host locks while
	// applying forwarded block-store config, so MountSpaceContents must return
	// before waiting for that startup path.
	conf := &plugin_space.Config{
		SpaceId:       spaceID,
		EngineId:      engineID,
		SessionPeerId: r.sessionPeerID,
		WorldBucketId: worldBucketID,
		HostPluginId:  hostPluginID,
	}

	id, err := resourceCtx.AddResource(contentsResource.GetMux(), contentsResource.Release)
	if err != nil {
		r.le.WithError(err).Info("failed to mount space contents: could not add resource")
		return nil, err
	}
	contentsResource.StartController(conf)
	r.le.
		WithField("space-id", spaceID).
		WithField("resource-id", id).
		Debug("fast-forward space contents")

	return &s4wave_space.MountSpaceContentsResponse{ResourceId: id}, nil
}

// _ is a type assertion
var _ s4wave_space.SRPCSpaceResourceServiceServer = ((*SpaceResource)(nil))

func bridgeSharingMailbox(
	ctx context.Context,
	state *sharingstate.State,
	swAcc *provider_spacewave.ProviderAccount,
	soID string,
) {
	accountBcast := swAcc.GetAccountBroadcast()
	for {
		var waitCh <-chan struct{}
		accountBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			waitCh = getWaitCh()
		})
		select {
		case <-ctx.Done():
			return
		case <-waitCh:
		}
		entries, _ := swAcc.GetPendingMailboxEntriesSnapshot(soID)
		state.SetMailboxEntries(sharingMailboxEntriesFromProto(entries))
	}
}

func sharingStateToProto(state *sharingstate.SharingState) *s4wave_space.SpaceSharingState {
	if state == nil {
		return nil
	}
	return &s4wave_space.SpaceSharingState{
		Participants:    state.Participants,
		Invites:         state.Invites,
		MailboxEntries:  sharingMailboxEntriesToProto(state.MailboxEntries),
		ViewerRole:      state.ViewerRole,
		CanManage:       state.CanManage,
		ParticipantInfo: sharingParticipantInfoToProto(state.ParticipantInfo),
	}
}

func sharingMailboxEntriesFromProto(
	entries []*s4wave_provider_spacewave.MailboxEntryInfo,
) []*sharingstate.MailboxEntry {
	if len(entries) == 0 {
		return nil
	}
	out := make([]*sharingstate.MailboxEntry, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			out = append(out, nil)
			continue
		}
		out = append(out, &sharingstate.MailboxEntry{
			ID:        entry.GetId(),
			InviteID:  entry.GetInviteId(),
			PeerID:    entry.GetPeerId(),
			Status:    entry.GetStatus(),
			CreatedAt: entry.GetCreatedAt(),
			AccountID: entry.GetAccountId(),
			EntityID:  entry.GetEntityId(),
		})
	}
	return out
}

func sharingMailboxEntriesToProto(
	entries []*sharingstate.MailboxEntry,
) []*s4wave_provider_spacewave.MailboxEntryInfo {
	if len(entries) == 0 {
		return nil
	}
	out := make([]*s4wave_provider_spacewave.MailboxEntryInfo, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			out = append(out, nil)
			continue
		}
		out = append(out, &s4wave_provider_spacewave.MailboxEntryInfo{
			Id:        entry.ID,
			InviteId:  entry.InviteID,
			PeerId:    entry.PeerID,
			Status:    entry.Status,
			CreatedAt: entry.CreatedAt,
			AccountId: entry.AccountID,
			EntityId:  entry.EntityID,
		})
	}
	return out
}

func sharingParticipantInfoToProto(
	info []*sharingstate.ParticipantInfo,
) []*s4wave_space.SpaceParticipantInfo {
	if len(info) == 0 {
		return nil
	}
	out := make([]*s4wave_space.SpaceParticipantInfo, 0, len(info))
	for _, row := range info {
		if row == nil {
			out = append(out, nil)
			continue
		}
		out = append(out, &s4wave_space.SpaceParticipantInfo{
			AccountId: row.AccountID,
			EntityId:  row.EntityID,
			PeerIds:   row.PeerIDs,
			Role:      row.Role,
			IsSelf:    row.IsSelf,
		})
	}
	return out
}

func loadSharingParticipantPresentationState(
	ctx context.Context,
	le *logrus.Entry,
	swAcc *provider_spacewave.ProviderAccount,
	soID string,
) *sharingstate.ParticipantPresentation {
	state := &sharingstate.ParticipantPresentation{}
	if swAcc == nil {
		return state
	}

	if accountState := swAcc.AccountStateSnapshot(); accountState != nil {
		state.SelfAccountID = accountState.GetAccountId()
		state.SelfEntityID = accountState.GetEntityId()
	}

	if soID == "" {
		return state
	}

	orgID, ok := swAcc.GetCachedSharedObjectOrganizationID(soID)
	if !ok {
		meta, err := swAcc.GetSharedObjectMetadata(ctx, soID)
		if err != nil {
			le.WithError(err).WithField("so-id", soID).Warn("failed to load space metadata for participant presentation")
			return state
		}
		if meta.GetOwnerType() != sobject.OwnerTypeOrganization || meta.GetOwnerId() == "" {
			return state
		}
		orgID = meta.GetOwnerId()
	}

	orgInfo, _, _, err := swAcc.GetOrganizationSnapshot(ctx, orgID)
	if err != nil {
		le.WithError(err).WithField("org-id", orgID).Warn("failed to load organization snapshot for participant presentation")
		return state
	}
	if len(orgInfo.GetMembers()) == 0 {
		return state
	}

	state.AccountLabels = make(map[string]string, len(orgInfo.GetMembers()))
	for _, member := range orgInfo.GetMembers() {
		accountID := member.GetSubjectId()
		entityID := member.GetEntityId()
		if accountID == "" || entityID == "" {
			continue
		}
		state.AccountLabels[accountID] = entityID
	}
	return state
}
