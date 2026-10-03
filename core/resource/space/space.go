package resource_space

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/bldr/storage"
	process_binding "github.com/s4wave/spacewave/core/plugin/process"
	plugin_space "github.com/s4wave/spacewave/core/plugin/space"
	plugin_space_runtime "github.com/s4wave/spacewave/core/plugin/space/runtime"
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
	// appPluginIDs is the immutable application declaration supplied before publication.
	appPluginIDs []string
	// bindingRegistry carries binding changes from the Resource root.
	bindingRegistry *process_binding.BindingRegistry
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
	)
	return spaceResource
}

// MountedBodySessionPeerID returns the session peer ID a SpaceResource over
// body acts as. A body whose SharedObject has no local peer is read-only, so
// it acts as no session peer.
func MountedBodySessionPeerID(body space.SpaceSharedObjectBody, sessionPeerID string) string {
	if body == nil {
		return sessionPeerID
	}
	if so := body.GetSharedObject(); so != nil && so.GetPeerID() == "" {
		return ""
	}
	return sessionPeerID
}

// resolveHostPluginID returns the configured host plugin ID, or the plugin ID
// of the host serving ctx when none is configured.
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
				return errors.Wrap(err, "open world transaction")
			}
			defer wtx.Discard()

			prevWorldSeqno, err = wtx.GetSeqno(ctx)
			if err != nil {
				return errors.Wrap(err, "read world sequence")
			}

			// Start a ready SpaceState response.
			state = &s4wave_space.SpaceState{Ready: true, EngineId: r.space.GetWorldEngineID()}

			// Load SpaceSettings when present.
			state.Settings, err = space_world.LookupSpaceSettingsBody(ctx, wtx)
			if err != nil {
				return errors.Wrap(err, "read Space settings")
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
// All change sources (SO state, mailbox metadata, join requests) are folded into one local
// broadcast so the watch loop reads every input snapshot under the same
// HoldLock that obtains the wait channel. This eliminates the missed-wakeup
// race that the previous dual-channel select had to defend against with
// per-source buffered signals, and coalesces near-simultaneous source
// changes into a single emission instead of one emission per source.
func (r *SpaceResource) WatchSpaceSharingState(
	req *s4wave_space.WatchSpaceSharingStateRequest,
	strm s4wave_space.SRPCSpaceResourceService_WatchSpaceSharingStateStream,
) error {
	// Only a shared object that hosts invitations has sharing state.
	ctx := strm.Context()
	inviteHost, ok := r.space.GetSharedObject().(sobject.InviteHost)
	if !ok {
		return nil
	}

	// Watch the shared object state.
	soHost := inviteHost.GetSOHost()
	soStateCtr, relSoStateCtr, err := soHost.GetSOStateCtr(ctx, nil)
	if err != nil {
		return err
	}
	defer relSoStateCtr()

	// A peer whose leave the head ownership transfer carries has left in its own view.
	readDeparting := func(ctx context.Context, state *sobject.SOState) ([]string, error) {
		return sobject.ReadSODepartingPeers(ctx, soHost, state.GetConfig().GetConfigChainHash())
	}
	soState, err := soStateCtr.WaitValue(ctx, nil)
	if err != nil {
		return err
	}
	departing, err := readDeparting(ctx, soState)
	if err != nil {
		return err
	}

	// Prime the mailbox cache and participant labels from the account.
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

	// Fold the first snapshot of every source into one state.
	var mailboxEntries []*sharingstate.MailboxEntry
	if swAcc != nil {
		snapshot, _ := swAcc.GetPendingMailboxEntriesSnapshot(soID)
		mailboxEntries = sharingMailboxEntriesFromProto(snapshot)
	}
	state := sharingstate.NewState(soState, departing, mailboxEntries, presentationState)

	// Bridge later source changes into the state.
	bridgeCtx, cancelBridges := context.WithCancel(ctx)
	defer cancelBridges()
	go state.BridgeSOState(bridgeCtx, soStateCtr, readDeparting)
	if swAcc != nil {
		go bridgeSharingMailbox(bridgeCtx, state, swAcc, soID)
	}
	if jrh, ok := inviteHost.(sobject.JoinRequestHost); ok {
		go state.BridgeJoinRequests(bridgeCtx, jrh.GetJoinRequestsCtr())
	}

	// Send each changed snapshot to the stream.
	peerID := r.space.GetSharedObject().GetPeerID().String()
	choices := sequencerChoices(r.space.GetSharedObject())
	_, controlHost := r.space.GetSharedObject().(sobject.ControlHost)
	return state.RunWatchLoop(ctx, peerID, func(state *sharingstate.SharingState) error {
		return strm.Send(sharingStateToProto(state, choices, controlHost))
	})
}

// sequencerChoices returns the sequencers an owner can appoint through so.
func sequencerChoices(so sobject.SharedObject) []s4wave_space.SpaceSequencer {
	// Only an owner-capable shared object offers a choice.
	if _, ok := so.(sobject.SequencerHost); !ok {
		return nil
	}

	// Merge is always offered, with each sequencer the shared object supports.
	choices := []s4wave_space.SpaceSequencer{s4wave_space.SpaceSequencer_SpaceSequencer_MERGE}
	if _, ok := so.(sobject.ProviderSequencer); ok {
		choices = append(choices, s4wave_space.SpaceSequencer_SpaceSequencer_PROVIDER)
	}
	if _, ok := so.(sobject.MainDevice); ok {
		choices = append(choices, s4wave_space.SpaceSequencer_SpaceSequencer_THIS_DEVICE)
	}
	return choices
}

// SetSpaceSequencer chooses, as the local owner, who orders the Space's edits.
func (r *SpaceResource) SetSpaceSequencer(
	ctx context.Context,
	req *s4wave_space.SetSpaceSequencerRequest,
) (*s4wave_space.SetSpaceSequencerResponse, error) {
	// Only a shared object whose owner can choose has a sequencer setting.
	so := r.space.GetSharedObject()
	host, ok := so.(sobject.SequencerHost)
	if !ok {
		return nil, errors.New("this Space has no sequencer setting")
	}

	// Resolve the chosen sequencer's peer ID.
	var peerID string
	switch choice := req.GetSequencer(); choice {
	case s4wave_space.SpaceSequencer_SpaceSequencer_MERGE:
	case s4wave_space.SpaceSequencer_SpaceSequencer_PROVIDER:
		ps, ok := so.(sobject.ProviderSequencer)
		if !ok {
			return nil, errors.New("this Space's provider cannot order edits")
		}
		var err error
		peerID, err = ps.GetProviderSequencer(ctx)
		if err != nil {
			return nil, err
		}
	case s4wave_space.SpaceSequencer_SpaceSequencer_THIS_DEVICE:
		if _, ok := so.(sobject.MainDevice); !ok {
			return nil, errors.New("this device cannot order this Space's edits")
		}
		peerID = so.GetPeerID().String()
	default:
		return nil, errors.Errorf("unsupported sequencer %v", choice)
	}

	// Appoint it, or agree to appoint it under group control.
	changed, err := host.SetSequencer(ctx, peerID)
	if errors.Is(err, sobject.ErrAwaitingGroup) {
		return &s4wave_space.SetSpaceSequencerResponse{AwaitingGroup: true}, nil
	}
	if err != nil {
		return nil, err
	}
	return &s4wave_space.SetSpaceSequencerResponse{Changed: changed}, nil
}

// SetSpaceControl chooses who controls the Space. An owner hands control to
// the group at once; under group control the viewer agrees to the change and
// the group decides it.
func (r *SpaceResource) SetSpaceControl(
	ctx context.Context,
	req *s4wave_space.SetSpaceControlRequest,
) (*s4wave_space.SetSpaceControlResponse, error) {
	// Only a shared object whose peer can change its control has the setting.
	host, ok := r.space.GetSharedObject().(sobject.ControlHost)
	if !ok {
		return nil, errors.New("this Space has no control setting")
	}

	// Resolve the choice.
	var control sobject.SOControl
	switch choice := req.GetControl(); choice {
	case s4wave_space.SpaceControl_SpaceControl_OWNER:
		control = sobject.SOControl_SO_CONTROL_OWNER
	case s4wave_space.SpaceControl_SpaceControl_GROUP:
		control = sobject.SOControl_SO_CONTROL_GROUP
	default:
		return nil, errors.Errorf("unsupported control %v", choice)
	}

	// Change it, or agree to change it under group control.
	err := host.SetControl(ctx, control)
	if errors.Is(err, sobject.ErrAwaitingGroup) {
		return &s4wave_space.SetSpaceControlResponse{AwaitingGroup: true}, nil
	}
	if err != nil {
		return nil, err
	}
	return &s4wave_space.SetSpaceControlResponse{}, nil
}

// ApproveSpaceChange agrees, as a voter, to a change another voter asked the
// group for.
func (r *SpaceResource) ApproveSpaceChange(
	ctx context.Context,
	req *s4wave_space.ApproveSpaceChangeRequest,
) (*s4wave_space.ApproveSpaceChangeResponse, error) {
	host, ok := r.space.GetSharedObject().(sobject.ControlHost)
	if !ok {
		return nil, errors.New("this Space has no group to agree with")
	}
	if err := host.ApproveConfigChange(ctx, req.GetHash()); err != nil {
		return nil, err
	}
	return &s4wave_space.ApproveSpaceChangeResponse{}, nil
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
	worldResource, err := r.NewWorldEngineResource()
	if err != nil {
		return nil, err
	}
	id, err := resourceCtx.AddResourceValue(worldResource.GetMux(), worldResource, worldResource.Close)
	if err != nil {
		worldResource.Close()
		return nil, err
	}

	return &s4wave_space.AccessWorldResponse{ResourceId: id}, nil
}

// NewWorldEngineResource carries this mounted Space's session authority into an
// Engine capability. The receiving Resource connection owns the capability.
func (r *SpaceResource) NewWorldEngineResource() (*resource_world.EngineResource, error) {
	sessionPeerID := peer.ID("")
	if r.sessionPeerID != "" {
		var err error
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
	return resource_world.NewEngineResource(
		r.le,
		r.b,
		r.space.GetWorldEngine(),
		lookupOp,
		engineInfo,
		resource_world.WithSessionPeerID(sessionPeerID),
	), nil
}

// MountSpaceContents mounts the shared plugin runtime of the space and returns a
// sub-resource for monitoring plugin status. Mounts of the same Space share one
// runtime, which stops after the last mount is released.
func (r *SpaceResource) MountSpaceContents(
	ctx context.Context,
	req *s4wave_space.MountSpaceContentsRequest,
) (*s4wave_space.MountSpaceContentsResponse, error) {
	// Find the caller's resource client.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		r.le.WithError(err).Info("failed to mount space contents: missing client context")
		return nil, err
	}

	// Acquire the shared runtime.
	conf, runtime, runtimeRef, err := r.AcquireContentsRuntime(ctx)
	if err != nil {
		r.le.WithError(err).Info("failed to mount space contents: could not start runtime")
		return nil, err
	}

	// Serve the contents resource, which holds the runtime until released.
	contentsResource := NewSpaceContentsResource(
		r.le,
		r.b,
		r.space.GetWorldEngine(),
		conf.GetSpaceId(),
		conf.GetEngineId(),
		runtime,
		runtimeRef,
	)
	id, err := resourceCtx.AddResource(contentsResource.GetMux(), contentsResource.Release)
	if err != nil {
		contentsResource.Release()
		r.le.WithError(err).Info("failed to mount space contents: could not add resource")
		return nil, err
	}
	r.le.
		WithField("space-id", conf.GetSpaceId()).
		WithField("resource-id", id).
		Debug("mounted space contents")
	return &s4wave_space.MountSpaceContentsResponse{ResourceId: id}, nil
}

// AcquireContentsRuntime acquires the shared plugin runtime of the Space.
//
// Every holder acquiring it through a SpaceResource with the same settings
// shares one runtime, which stops after the last reference is released. It
// returns before the runtime's startup completes, because startup may need
// document-owned plugin-host locks while applying forwarded block-store config.
func (r *SpaceResource) AcquireContentsRuntime(ctx context.Context) (
	*plugin_space.Config,
	*plugin_space_runtime.Controller,
	directive.Reference,
	error,
) {
	// Start or join the runtime keyed by the shared config.
	conf := r.contentsRuntimeConfig(ctx)
	runtime, runtimeRef, err := plugin_space_runtime.StartControllerWithConfig(
		ctx,
		r.b,
		&plugin_space_runtime.Config{Space: conf, AppPluginIds: r.appPluginIDs},
	)
	if err != nil {
		return nil, nil, nil, err
	}

	// Report process binding changes to the Resource root's watches.
	if r.bindingRegistry != nil {
		runtime.SetBindingRegistry(r.bindingRegistry)
	}
	return conf, runtime, runtimeRef, nil
}

// contentsRuntimeConfig builds the plugin/space config shared by every contents
// mount of the Space.
func (r *SpaceResource) contentsRuntimeConfig(ctx context.Context) *plugin_space.Config {
	// The Space resource runs inside a plugin host; cloud block store forwarding
	// registers under that host's plugin service prefix. Without a host plugin
	// ID, skip forwarding rather than registering under a guessed prefix.
	hostPluginID := r.resolveHostPluginID(ctx)
	worldBucketID := r.space.GetWorldEngineBucketID()
	if hostPluginID == "" {
		worldBucketID = ""
	}
	return &plugin_space.Config{
		SpaceId:       space.SpaceEngineId(r.space.GetSharedObjectRef()),
		VolumeId:      bldr_plugin.PluginVolumeID,
		ObjectStoreId: "platform-account",
		EngineId:      r.space.GetWorldEngineID(),
		SessionPeerId: r.sessionPeerID,
		WorldBucketId: worldBucketID,
		HostPluginId:  hostPluginID,
		HostStorageId: storage.GetHostStorageID(ctx),
	}
}

// bridgeSharingMailbox copies the pending mailbox entries of soID into state on
// every account change until ctx ends.
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

// sharingStateToProto converts the sharing state to its wire form, with the
// sequencers an owner can appoint and whether the shared object's control can
// change.
func sharingStateToProto(
	state *sharingstate.SharingState,
	choices []s4wave_space.SpaceSequencer,
	controlHost bool,
) *s4wave_space.SpaceSharingState {
	// No sharing state projects to none.
	if state == nil {
		return nil
	}

	// Under group control a voter asks for changes; otherwise an owner makes them.
	group := state.Control == sobject.SOControl_SO_CONTROL_GROUP
	canVote := state.ViewerWeight != 0
	control := s4wave_space.SpaceControl_SpaceControl_OWNER
	var quorum uint64
	if group {
		control = s4wave_space.SpaceControl_SpaceControl_GROUP
		quorum = state.TotalWeight*2/3 + 1
	}
	changes := make([]*s4wave_space.SpaceGroupChange, 0, len(state.Agreements))
	for _, a := range state.Agreements {
		changes = append(changes, sharingGroupChange(state, a, choices))
	}

	return &s4wave_space.SpaceSharingState{
		Participants:     state.Participants,
		Invites:          state.Invites,
		MailboxEntries:   sharingMailboxEntriesToProto(state.MailboxEntries),
		JoinRequests:     state.JoinRequests,
		ViewerRole:       state.ViewerRole,
		CanManage:        state.CanManage,
		ParticipantInfo:  sharingParticipantInfoToProto(state.ParticipantInfo),
		ConfigChainHash:  state.ConfigChainHash,
		ConfigChainSeqno: state.ConfigChainSeqno,
		ViewerPeerId:     state.ViewerPeerID,
		DeparturePending: state.DeparturePending,
		Sequencer:        sharingSequencer(state, state.SequencerPeerID, choices),
		SequencerPeerId:  state.SequencerPeerID,
		SequencerChoices: choices,
		Control:          control,
		CanVote:          canVote,
		CanSetControl:    controlHost && (group && canVote || !group && state.CanManage),
		TotalWeight:      state.TotalWeight,
		QuorumWeight:     quorum,
		GroupChanges:     changes,
	}
}

// sharingGroupChange describes an agreed change by what it does to the held
// config: the peers it admits or removes, the control and the sequencer.
func sharingGroupChange(
	state *sharingstate.SharingState,
	a *sobject.ControlAgreement,
	choices []s4wave_space.SpaceSequencer,
) *s4wave_space.SpaceGroupChange {
	// Compare the participants before and after.
	next := a.Change.GetConfig()
	hasPeer := func(ps []*sobject.SOParticipantConfig, peerID string) bool {
		return slices.ContainsFunc(ps, func(p *sobject.SOParticipantConfig) bool { return p.GetPeerId() == peerID })
	}
	var added, removed []string
	for _, p := range next.GetParticipants() {
		if !hasPeer(state.Participants, p.GetPeerId()) {
			added = append(added, p.GetPeerId())
		}
	}
	for _, p := range state.Participants {
		if !hasPeer(next.GetParticipants(), p.GetPeerId()) {
			removed = append(removed, p.GetPeerId())
		}
	}

	// Describe the control and the sequencer after the change.
	control := s4wave_space.SpaceControl_SpaceControl_OWNER
	if next.IsGroupControl() {
		control = s4wave_space.SpaceControl_SpaceControl_GROUP
	}
	return &s4wave_space.SpaceGroupChange{
		Hash:           a.Hash,
		ChangeType:     a.Change.GetChangeType(),
		AddedPeerIds:   added,
		RemovedPeerIds: removed,
		Control:        control,
		Sequencer:      sharingSequencer(state, next.GetSequencer().GetPeerId(), choices),
		Weight:         a.Weight,
		ViewerAgreed:   slices.Contains(a.Voters, state.ViewerPeerID),
	}
}

// sharingSequencer classifies the sequencer peerID for the viewer. A
// sequencer that is not a participant is the provider's when the provider can
// sequence, and otherwise a device that has left.
func sharingSequencer(
	state *sharingstate.SharingState,
	peerID string,
	choices []s4wave_space.SpaceSequencer,
) s4wave_space.SpaceSequencer {
	switch {
	case peerID == "":
		return s4wave_space.SpaceSequencer_SpaceSequencer_MERGE
	case peerID == state.ViewerPeerID:
		return s4wave_space.SpaceSequencer_SpaceSequencer_THIS_DEVICE
	case slices.ContainsFunc(state.Participants, func(p *sobject.SOParticipantConfig) bool {
		return p.GetPeerId() == peerID
	}):
		return s4wave_space.SpaceSequencer_SpaceSequencer_OTHER_DEVICE
	case slices.Contains(choices, s4wave_space.SpaceSequencer_SpaceSequencer_PROVIDER):
		return s4wave_space.SpaceSequencer_SpaceSequencer_PROVIDER
	default:
		return s4wave_space.SpaceSequencer_SpaceSequencer_OTHER_DEVICE
	}
}

// sharingMailboxEntriesFromProto converts provider mailbox entries to sharing
// state entries.
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

// sharingMailboxEntriesToProto converts sharing state mailbox entries to their
// provider wire form.
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

// sharingParticipantInfoToProto converts sharing participants to their wire
// form.
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

// loadSharingParticipantPresentationState loads the account and organization
// identity used to present Space participants. It returns a partial state when
// metadata is unavailable.
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

// _ is a type assertion
var _ s4wave_space.SRPCSpaceResourceServiceServer = (*SpaceResource)(nil)

// SetAppPluginIDs supplies application composition before the resource is published.
func (r *SpaceResource) SetAppPluginIDs(ids []string) {
	r.appPluginIDs = slices.Clone(ids)
}

// SetBindingRegistry supplies the Resource root's binding event source.
func (r *SpaceResource) SetBindingRegistry(registry *process_binding.BindingRegistry) {
	r.bindingRegistry = registry
}
