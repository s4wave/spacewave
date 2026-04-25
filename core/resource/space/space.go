package resource_space

import (
	"context"
	"sort"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/ccontainer"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	plugin_space "github.com/s4wave/spacewave/core/plugin/space"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/world"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	s4wave_wizard "github.com/s4wave/spacewave/sdk/world/wizard"
	"github.com/sirupsen/logrus"
)

// SpaceResource wraps a Space for resource access.
type SpaceResource struct {
	le    *logrus.Entry
	b     bus.Bus
	mux   srpc.Invoker
	space space.SpaceSharedObjectBody
}

// NewSpaceResource creates a new SpaceResource.
func NewSpaceResource(le *logrus.Entry, b bus.Bus, sp space.SpaceSharedObjectBody) *SpaceResource {
	spaceResource := &SpaceResource{le: le, b: b, space: sp}
	spaceResource.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		if err := s4wave_space.SRPCRegisterSpaceResourceService(mux, spaceResource); err != nil {
			return err
		}
		wizardResource := s4wave_wizard.NewWizardRegistryResource()
		return s4wave_wizard.SRPCRegisterObjectWizardRegistryResourceService(mux, wizardResource)
	})
	return spaceResource
}

// GetMux returns the rpc mux.
func (r *SpaceResource) GetMux() srpc.Invoker {
	return r.mux
}

// WatchSpaceState watches the SpaceState for the component.
func (r *SpaceResource) WatchSpaceState(
	req *s4wave_space.WatchSpaceStateRequest,
	strm s4wave_space.SRPCSpaceResourceService_WatchSpaceStateStream,
) error {
	ctx, worldEng := strm.Context(), r.space.GetWorldEngine()

	// Watch the world contents.
	var prevWorldSeqno uint64
	for {
		r.le.Debugf("checking world state: seqno(%v)", prevWorldSeqno+1)
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

			state = &s4wave_space.SpaceState{Ready: true}

			// build the object list
			state.WorldContents, err = space_world.BuildWorldContents(ctx, wtx)
			if err != nil {
				return err
			}

			// Load SpaceSettings from the world (ignore not found error)
			state.Settings, _, err = space_world.LookupSpaceSettings(ctx, wtx)
			if err != nil {
				return err
			}

			// Build transform info from the shared object state.
			state.TransformInfo = r.buildTransformInfo(ctx)

			// send ready
			return strm.Send(state)
		}(); err != nil {
			return err
		}

		// wait til seqno changes
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

	state := &sharingWatchState{soState: soState}
	if swAcc != nil {
		state.mailboxEntries, _ = swAcc.GetPendingMailboxEntriesSnapshot(soID)
	}
	state.participantPresentation = presentationState

	bridgeCtx, cancelBridges := context.WithCancel(ctx)
	defer cancelBridges()
	go state.bridgeSOState(bridgeCtx, soStateCtr)
	if swAcc != nil {
		go state.bridgeMailbox(bridgeCtx, swAcc, soID)
	}

	peerID := r.space.GetSharedObject().GetPeerID().String()
	return state.runWatchLoop(ctx, peerID, strm.Send)
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

	lookupOp := world.BuildLookupWorldOpFunc(r.b, r.le, r.space.GetWorldEngineID())
	engineInfo := &s4wave_world.EngineInfo{
		EngineId: r.space.GetWorldEngineID(),
		BucketId: r.space.GetWorldEngineBucketID(),
	}
	worldResource := resource_world.NewEngineResource(r.le, r.b, r.space.GetWorldEngine(), lookupOp, engineInfo)
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
		return nil, err
	}

	ref := r.space.GetSharedObjectRef()
	spaceID := space.SpaceEngineId(ref)
	engineID := r.space.GetWorldEngineID()

	// Create the contents sub-resource.
	contentsResource := NewSpaceContentsResource(r.le, r.b, r.space.GetWorldEngine(), spaceID, engineID)

	// Start the plugin/space controller so plugins load immediately.
	conf := &plugin_space.Config{
		SpaceId:       spaceID,
		EngineId:      engineID,
		SessionPeerId: r.space.GetSharedObject().GetPeerID().String(),
	}
	ctrl, _, ctrlRef, err := plugin_space.StartControllerWithConfig(ctx, r.b, conf, func() {})
	if err != nil {
		return nil, err
	}
	contentsResource.ctrl = ctrl
	contentsResource.ctrlRef = ctrlRef

	id, err := resourceCtx.AddResource(contentsResource.GetMux(), contentsResource.Release)
	if err != nil {
		return nil, err
	}

	return &s4wave_space.MountSpaceContentsResponse{ResourceId: id}, nil
}

// _ is a type assertion
var _ s4wave_space.SRPCSpaceResourceServiceServer = ((*SpaceResource)(nil))

// sharingWatchState carries every input snapshot the sharing watch reads
// per emission, guarded by a single broadcast so the watch loop reads all
// of them under the same HoldLock that obtains the wait channel. Both
// bridge goroutines update fields under HoldLock and broadcast on change;
// the watch never observes a stale wait channel paired with a fresh source
// update.
type sharingWatchState struct {
	soState                 *sobject.SOState
	mailboxEntries          []*s4wave_provider_spacewave.MailboxEntryInfo
	participantPresentation *sharingParticipantPresentationState
	err                     error
	bcast                   broadcast.Broadcast
}

// bridgeSOState forwards SO state container updates into the local broadcast.
func (s *sharingWatchState) bridgeSOState(
	ctx context.Context,
	soStateCtr ccontainer.Watchable[*sobject.SOState],
) {
	current := s.soState
	for {
		next, err := soStateCtr.WaitValueChange(ctx, current, nil)
		if err != nil {
			if ctx.Err() == nil {
				s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
					if s.err == nil {
						s.err = err
					}
					broadcast()
				})
			}
			return
		}
		current = next
		s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			s.soState = next
			broadcast()
		})
	}
}

// bridgeMailbox forwards account-broadcast wakeups into the local broadcast,
// snapshotting the per-SO mailbox entries on each update so the watch loop
// reads them under HoldLock.
func (s *sharingWatchState) bridgeMailbox(
	ctx context.Context,
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
		s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			s.mailboxEntries = entries
			broadcast()
		})
	}
}

// runWatchLoop emits a fresh SpaceSharingState whenever any folded source
// changes. The state and wait channel are read in the same HoldLock so a
// source update that lands between reading state and selecting on the wait
// channel cannot be missed: the broadcast in the source bridge replaces
// the wait channel before the watch loop blocks on it.
func (s *sharingWatchState) runWatchLoop(
	ctx context.Context,
	peerID string,
	send func(*s4wave_space.SpaceSharingState) error,
) error {
	var prevResp *s4wave_space.SpaceSharingState
	for {
		var (
			resp      *s4wave_space.SpaceSharingState
			bridgeErr error
			waitCh    <-chan struct{}
		)
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			bridgeErr = s.err
			viewerRole := getViewerRole(s.soState, peerID)
			resp = &s4wave_space.SpaceSharingState{
				Participants:   s.soState.GetConfig().GetParticipants(),
				Invites:        s.soState.GetInvites(),
				MailboxEntries: s.mailboxEntries,
				ViewerRole:     viewerRole,
				CanManage:      sobject.IsOwner(viewerRole),
				ParticipantInfo: buildSpaceParticipantInfo(
					s.soState,
					peerID,
					s.participantPresentation,
				),
			}
			waitCh = getWaitCh()
		})
		if bridgeErr != nil {
			return bridgeErr
		}
		if prevResp == nil || !resp.EqualVT(prevResp) {
			if err := send(resp); err != nil {
				return err
			}
			prevResp = resp
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
		}
	}
}

// getViewerRole returns the current viewer's effective participant role.
func getViewerRole(state *sobject.SOState, peerID string) sobject.SOParticipantRole {
	if state == nil || peerID == "" {
		return sobject.SOParticipantRole_SOParticipantRole_UNKNOWN
	}

	role := sobject.SOParticipantRole_SOParticipantRole_UNKNOWN
	for _, participant := range state.GetConfig().GetParticipants() {
		if participant.GetPeerId() != peerID {
			continue
		}
		if participant.GetRole() > role {
			role = participant.GetRole()
		}
	}
	return role
}

type sharingParticipantPresentationState struct {
	selfAccountID string
	selfEntityID  string
	accountLabels map[string]string
}

func loadSharingParticipantPresentationState(
	ctx context.Context,
	le *logrus.Entry,
	swAcc *provider_spacewave.ProviderAccount,
	soID string,
) *sharingParticipantPresentationState {
	state := &sharingParticipantPresentationState{}
	if swAcc == nil {
		return state
	}

	if accountState := swAcc.AccountStateSnapshot(); accountState != nil {
		state.selfAccountID = accountState.GetAccountId()
		state.selfEntityID = accountState.GetEntityId()
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

	state.accountLabels = make(map[string]string, len(orgInfo.GetMembers()))
	for _, member := range orgInfo.GetMembers() {
		accountID := member.GetSubjectId()
		entityID := member.GetEntityId()
		if accountID == "" || entityID == "" {
			continue
		}
		state.accountLabels[accountID] = entityID
	}
	return state
}

func buildSpaceParticipantInfo(
	soState *sobject.SOState,
	selfPeerID string,
	presentation *sharingParticipantPresentationState,
) []*s4wave_space.SpaceParticipantInfo {
	if soState == nil || soState.GetConfig() == nil {
		return nil
	}

	participants := soState.GetConfig().GetParticipants()
	if len(participants) == 0 {
		return nil
	}

	rows := make(map[string]*s4wave_space.SpaceParticipantInfo, len(participants))
	keys := make([]string, 0, len(participants))
	for _, participant := range participants {
		peerID := participant.GetPeerId()
		if peerID == "" {
			continue
		}

		accountID := participant.GetEntityId()
		key := accountID
		if key == "" {
			key = "peer:" + peerID
		}

		row := rows[key]
		if row == nil {
			row = &s4wave_space.SpaceParticipantInfo{
				AccountId: accountID,
				Role:      participant.GetRole(),
			}
			if accountID != "" && presentation != nil {
				if label := presentation.accountLabels[accountID]; label != "" {
					row.EntityId = label
				}
				if accountID == presentation.selfAccountID && presentation.selfEntityID != "" {
					row.EntityId = presentation.selfEntityID
				}
			}
			rows[key] = row
			keys = append(keys, key)
		}

		if participant.GetRole() > row.GetRole() {
			row.Role = participant.GetRole()
		}
		row.PeerIds = append(row.GetPeerIds(), peerID)
		if peerID == selfPeerID {
			row.IsSelf = true
		}
	}

	if len(keys) == 0 {
		return nil
	}

	sort.SliceStable(keys, func(i, j int) bool {
		return participantSortLabel(rows[keys[i]]) < participantSortLabel(rows[keys[j]])
	})

	out := make([]*s4wave_space.SpaceParticipantInfo, 0, len(keys))
	for _, key := range keys {
		out = append(out, rows[key])
	}
	return out
}

func participantSortLabel(info *s4wave_space.SpaceParticipantInfo) string {
	if info == nil {
		return ""
	}
	if info.GetEntityId() != "" {
		return info.GetEntityId()
	}
	if info.GetAccountId() != "" {
		return info.GetAccountId()
	}
	if len(info.GetPeerIds()) != 0 {
		return info.GetPeerIds()[0]
	}
	return ""
}
