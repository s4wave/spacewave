package sobject_invite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"slices"

	"github.com/aperturerobotics/util/csync"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
)

// server implements the SOInviteService SRPC server.
type server struct {
	// h performs the owner operations.
	h Handlers
	// acceptMtx serializes invite acceptance from lookup through the use
	// increment, so a limited-use invite cannot enroll more peers than it admits.
	acceptMtx csync.Mutex
}

// Leave binds the transport to a departing identity before invoking the native
// owner.
func (s *server) Leave(ctx context.Context, request *sobject.SOLeaveRequest) (*sobject.SOLeaveResponse, error) {
	// A valid proof from another connection cannot be relayed as fresh caller
	// authority.
	peers, err := request.Verify()
	if err != nil {
		return nil, err
	}
	stream := link.GetMountedStreamContext(ctx)
	if stream == nil || !slices.Contains(peers, stream.GetPeerID().String()) {
		return nil, errors.New("leave stream does not match a departing identity")
	}

	// The mounted provider retains and mutates its own host for the complete
	// operation.
	if s.h.Leave == nil {
		return nil, errors.New("voluntary departure is unavailable")
	}
	return s.h.Leave(ctx, request)
}

// WithdrawJoinRequest removes the stream peer's pending join request.
func (s *server) WithdrawJoinRequest(ctx context.Context, req *WithdrawJoinRequestRequest) (*WithdrawJoinRequestResponse, error) {
	// Validate the request.
	soID := req.GetSharedObjectId()
	if soID == "" {
		return nil, errors.New("shared_object_id is required")
	}

	// Only the authenticated requester withdraws its own request.
	stream := link.GetMountedStreamContext(ctx)
	if stream == nil {
		return nil, errors.New("no mounted stream context")
	}

	// A host that holds no join requests has none to remove.
	if s.h.Withdraw == nil {
		return &WithdrawJoinRequestResponse{}, nil
	}
	if err := s.h.Withdraw(ctx, soID, stream.GetPeerID()); err != nil {
		return nil, err
	}
	return &WithdrawJoinRequestResponse{}, nil
}

// AcceptInvite processes a join request from an invitee.
func (s *server) AcceptInvite(ctx context.Context, req *AcceptInviteRequest) (*AcceptInviteResponse, error) {
	// Validate the request.
	joinResp := req.GetJoinResponse()
	if joinResp == nil {
		return nil, errors.New("join_response is required")
	}
	token := req.GetToken()
	if len(token) == 0 {
		return nil, errors.New("token is required")
	}

	// Hash the raw token to look up the on-chain invite.
	// The invitee proves possession of the raw token. The on-chain state
	// stores only the SHA256 hash.
	tokenHashArr := sha256.Sum256(token)
	tokenHash := tokenHashArr[:]

	// Verify the invitee is who they say they are via mounted stream context.
	ms := link.GetMountedStreamContext(ctx)
	if ms == nil {
		return nil, errors.New("no mounted stream context")
	}
	streamPeerID := ms.GetPeerID()

	// Verify both join responses. The stream peer must be their author.
	redeemer, err := ValidateRedeemer(joinResp, req.GetStorageJoinResponse())
	if err != nil {
		return nil, err
	}
	responderPeerID := redeemer.PeerID
	if streamPeerID != responderPeerID {
		return nil, errors.New("stream peer ID does not match join response responder")
	}

	// Hold the acceptance lock so the usability check below observes every
	// prior acceptance's committed use.
	relAccept, err := s.acceptMtx.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer relAccept()

	// Look up the invite by token hash.
	result, err := s.h.Lookup(ctx, tokenHash)
	if err != nil {
		return nil, errors.Wrap(err, "look up invite")
	}
	if result == nil {
		return nil, errors.New("no matching invite found")
	}

	// Verify the token hash matches the on-chain invite.
	if !bytes.Equal(result.Invite.GetTokenHash(), tokenHash) {
		return nil, errors.New("token hash mismatch")
	}

	// Verify the invite ID in the join response matches.
	if joinResp.GetInviteId() != result.Invite.GetInviteId() {
		return nil, errors.New("invite ID mismatch")
	}

	// A redeemer the Space already enrolled at the invite's role receives its
	// grant, however the invite changed since: the repeat changes nothing.
	ownerState, err := result.Host.GetHostState(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "read owner shared object state")
	}
	ownerConfig := ownerState.GetConfig()
	if sobject.InviteEnrolled(ownerConfig, result.Invite, responderPeerID.String()) &&
		sobject.InviteEnrolled(ownerConfig, result.Invite, redeemer.StoragePeerID.String()) {
		role := ownerConfig.GetParticipantRole(responderPeerID.String())
		return s.answerAccept(ctx, result, responderPeerID, nil, sobject.SOParticipantAddOutcome_SOParticipantAddOutcome_UNCHANGED, role)
	}

	// Validate the responder may redeem the invite: it is the named peer of a
	// targeted invite, or the invite is untargeted, and the invite is live and
	// below its max_uses.
	if err := sobject.ValidateInviteRedeemable(result.Invite, responderPeerID.String()); err != nil {
		return nil, errors.Wrap(err, "invite not usable")
	}

	// Admit, queue or refuse the redeemer under the invite's conditions.
	held, err := s.readHeldConfigs(ctx, result.Invite)
	if err != nil {
		return nil, err
	}
	switch sobject.RedeemInvite(result.Invite, responderPeerID.String(), held) {
	case sobject.InviteRedemptionRefuse:
		return nil, errors.New("invite does not admit this peer")
	case sobject.InviteRedemptionQueue:
		if s.h.Queue == nil {
			return nil, errors.New("join requests are unavailable")
		}
		if err := s.h.Queue(ctx, result, joinResp, req.GetStorageJoinResponse()); err != nil {
			return nil, errors.Wrap(err, "queue join request")
		}
		return &AcceptInviteResponse{SharedObjectId: result.SharedObjectID, Pending: true}, nil
	}

	// Admit the redeemer and its storage peer.
	if s.h.Enroll == nil {
		return nil, errors.New("enrollment not configured")
	}
	grant, outcome, joinedRole, err := Admit(ctx, s.h.Enroll, result, redeemer)
	if err != nil {
		return nil, err
	}
	return s.answerAccept(ctx, result, responderPeerID, grant, outcome, joinedRole)
}

// answerAccept returns the redeemer's grant with the durable owner state and
// its config lineage. A nil grant is read from the current key epoch, for a
// redeemer that already held one.
func (s *server) answerAccept(
	ctx context.Context,
	result *InviteLookupResult,
	responderPeerID peer.ID,
	grant *sobject.SOGrant,
	outcome sobject.SOParticipantAddOutcome,
	role sobject.SOParticipantRole,
) (*AcceptInviteResponse, error) {
	// Read the owner state once every acceptance mutation is durable.
	ownerState, err := result.Host.GetHostState(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "read updated owner shared object state")
	}
	if err := result.Host.WaitDurable(ctx); err != nil {
		return nil, errors.Wrap(err, "wait for durable shared object state")
	}

	// A redeemer that already held a grant receives it from the owner state.
	if grant == nil {
		grant = ownerState.CurrentKeyEpoch().FindGrant(responderPeerID.String())
		if grant == nil {
			return nil, errors.New("participant exists without a current key epoch grant")
		}
	}

	// Return the grant and the outcome with the owner state and its lineage.
	lineage, err := result.Host.ReadConfigLineage(ctx, ownerState.GetConfig().GetConfigChainHash())
	if err != nil {
		return nil, errors.Wrap(err, "read shared object config lineage")
	}
	return &AcceptInviteResponse{
		Grant:             grant,
		SharedObjectId:    result.SharedObjectID,
		SharedObjectState: ownerState.Clone(),
		ConfigLineage:     lineage,
		Outcome:           outcome,
		ParticipantRole:   role,
	}, nil
}

// readHeldConfigs reads this host's configuration of each Space the invite
// selects.
func (s *server) readHeldConfigs(ctx context.Context, inv *sobject.SOInvite) (map[string]*sobject.SharedObjectConfig, error) {
	// Without named Spaces or a reader there is nothing to read.
	ids := inv.GetParticipantOf().GetSharedObjectIds()
	if len(ids) == 0 || s.h.HeldConfig == nil {
		return nil, nil
	}

	// Read each held Space's config, skipping Spaces this host does not hold.
	held := make(map[string]*sobject.SharedObjectConfig, len(ids))
	for _, id := range ids {
		config, err := s.h.HeldConfig(ctx, id)
		if err != nil {
			return nil, errors.Wrapf(err, "read held space %s", id)
		}
		if config != nil {
			held[id] = config
		}
	}
	return held, nil
}

// _ is a type assertion.
var _ SRPCSOInviteServiceServer = (*server)(nil)
