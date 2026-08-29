package resource_session

import (
	"context"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// localSpaceLinkInviteTTL is how long a one-use targeted invite from a local
// SpaceLink approval stays valid. It outlives the ticket so the Device can
// complete after the ticket itself has expired.
const localSpaceLinkInviteTTL = time.Hour

// ApproveSpaceLink verifies a signed SpaceLink Device ticket for a target
// Space, confirms the approving peer is OWNER, consumes the ticket nonce, and
// returns a one-use targeted invite for the Device to join. The local
// provider performs no cloud session registration and returns no account
// credential.
func (r *LocalSessionResource) ApproveSpaceLink(
	ctx context.Context,
	req *s4wave_session.ApproveLocalSpaceLinkRequest,
) (*s4wave_session.ApproveLocalSpaceLinkResponse, error) {
	verified, err := verifySpaceLinkTicketData(req.GetTicket(), time.Now())
	if err != nil {
		return nil, err
	}
	resourceID := req.GetResourceId()
	if resourceID == "" {
		return nil, errors.New("resource_id is required")
	}

	localAcc, ok := r.session.GetProviderAccount().(*provider_local.ProviderAccount)
	if !ok {
		return nil, errors.New("session provider account is not local")
	}

	// The authority check happens before any mutation. Only the local account
	// that originated the Space can enroll Devices. A joined copy initializes
	// its own storage signer as OWNER, so participant role alone cannot
	// distinguish the workstation authority from an enrolled Device.
	so, err := r.mountLocalSpace(ctx, localAcc, resourceID)
	if err != nil {
		return nil, err
	}
	defer so.release()

	ih, ok := any(so.so).(sobject.InviteHost)
	if !ok {
		return nil, errors.New("shared object does not support invites")
	}
	sessionPriv := r.session.GetPrivKey()
	if sessionPriv == nil {
		return nil, errors.New("session is locked")
	}
	approverPeerID, err := peer.IDFromPrivateKey(ih.GetPrivKey())
	if err != nil {
		return nil, errors.Wrap(err, "derive approver peer id")
	}
	if err := so.requireOriginOwner(ctx, approverPeerID.String()); err != nil {
		return nil, err
	}

	payload := verified.payload
	if err := localAcc.ConsumeSpaceLinkNonce(
		ctx,
		string(payload.GetAgentPeerId()),
		payload.GetNonce(),
		verified.ticket.GetPayload(),
		time.Unix(payload.GetExpiresAt(), 0),
	); err != nil {
		return nil, err
	}

	// The invite message is signed with the approving session's key so its
	// owner peer ID is the session transport peer that serves the SO invite
	// server. The one-use invite itself is stored on the Space signed by the
	// account's shared-object key, which holds the OWNER role.
	inviteMsg, invite, err := sobject.BuildSOInviteMessage(
		resourceID,
		sessionPriv,
		payload.GetRequestedRole(),
		ih.GetProviderID(),
		verified.agentPeerID.String(),
		1,
		timestamppb.New(time.Now().Add(localSpaceLinkInviteTTL)),
	)
	if err != nil {
		return nil, errors.Wrap(err, "build targeted invite")
	}
	if err := ih.GetSOHost().CreateInvite(ctx, ih.GetPrivKey(), invite); err != nil {
		return nil, errors.Wrap(err, "store targeted invite")
	}
	ownerTransport := localAcc.GetSessionTransport()
	if ownerTransport == nil {
		return nil, errors.New("approving session transport is not available")
	}
	if err := localAcc.StartP2PSync(context.WithoutCancel(ctx), ownerTransport); err != nil {
		return nil, errors.Wrap(err, "start invite service")
	}

	return &s4wave_session.ApproveLocalSpaceLinkResponse{
		Completion: &s4wave_session.LocalSpaceLinkCompletion{
			ProviderId:    ih.GetProviderID(),
			ResourceId:    resourceID,
			SessionPeerId: payload.GetAgentPeerId(),
			SessionType:   payload.GetSessionType(),
			Invite:        inviteMsg,
			Nonce:         payload.GetNonce(),
		},
	}, nil
}

// mountedLocalSpace is a mounted local Space shared object targeted by a
// SpaceLink approval.
type mountedLocalSpace struct {
	so         *provider_local.SharedObject
	providerID string
	sharedCopy bool
	release    func()
}

// requireOriginOwner returns an error unless the peer is an OWNER on the
// originating local account's copy of the Space.
func (m *mountedLocalSpace) requireOriginOwner(ctx context.Context, approverPeerID string) error {
	if m.sharedCopy {
		return errors.New("spacelink approval requires the originating local account")
	}
	if approverPeerID == "" {
		return errors.New("approver peer id is required")
	}
	state, err := m.so.GetSOHostState(ctx)
	if err != nil {
		return errors.Wrap(err, "get target space state")
	}
	for _, participant := range state.GetConfig().GetParticipants() {
		if participant.GetPeerId() == approverPeerID && sobject.IsOwner(participant.GetRole()) {
			return nil
		}
	}
	return errors.New("spacelink approver must be OWNER on the target space")
}

// mountLocalSpace mounts a local Space shared object by ID on the account.
// The Space must already belong to the account's shared object list; the
// approval never creates or adopts a Space the account does not hold.
func (r *LocalSessionResource) mountLocalSpace(
	ctx context.Context,
	localAcc *provider_local.ProviderAccount,
	resourceID string,
) (*mountedLocalSpace, error) {
	providerID := localAcc.GetProviderID()
	found := false
	sharedCopy := false
	for _, entry := range localAcc.GetSOListCtr().GetValue().GetSharedObjects() {
		if entry.GetRef().GetProviderResourceRef().GetId() == resourceID {
			found = true
			sharedCopy = entry.GetSource() == "shared"
			break
		}
	}
	if !found {
		return nil, errors.New("target space not found on the approving account")
	}
	ref := sobject.NewSharedObjectRef(
		providerID,
		localAcc.GetAccountID(),
		resourceID,
		provider_local.SobjectBlockStoreID(resourceID),
	)
	soIface, relSO, err := localAcc.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return nil, errors.Wrap(err, "mount target space")
	}
	so, ok := soIface.(*provider_local.SharedObject)
	if !ok {
		relSO()
		return nil, errors.New("unexpected shared object type")
	}
	return &mountedLocalSpace{
		so:         so,
		providerID: providerID,
		sharedCopy: sharedCopy,
		release:    relSO,
	}, nil
}
