package sobject_invite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"slices"

	"github.com/aperturerobotics/util/csync"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// InviteLookupFn resolves an invite by token hash.
// Returns nil result if no matching invite is found.
type InviteLookupFn func(ctx context.Context, tokenHash []byte) (*InviteLookupResult, error)

// EnrollFn enrolls a participant after invite verification.
// Called with the resolved invite context and the invitee's identity.
// Returns the SOGrant for the invitee.
type EnrollFn func(ctx context.Context, result *InviteLookupResult, inviteePeerID peer.ID, inviteePubKey crypto.PubKey) (*sobject.SOGrant, error)

// LeaveFn commits voluntary departure while retaining the selected native host through acknowledgment.
type LeaveFn func(context.Context, *sobject.SOLeaveRequest) (*sobject.SOLeaveResponse, error)

// QueueFn holds a redemption as a join request for an owner to grant or refuse.
type QueueFn func(ctx context.Context, result *InviteLookupResult, joinResp *sobject.SOJoinResponse) error

// WithdrawFn removes the pending join request of peerID to join the shared
// object, succeeding when none remains.
type WithdrawFn func(ctx context.Context, sharedObjectID string, peerID peer.ID) error

// HeldConfigFn returns the host's own configuration of a Space, or nil when
// the host does not hold it.
type HeldConfigFn func(ctx context.Context, sharedObjectID string) (*sobject.SharedObjectConfig, error)

// Handlers are the owner operations behind the invite service.
type Handlers struct {
	// Lookup resolves the owner's invitation authority.
	Lookup InviteLookupFn
	// Enroll issues participant grants under the resolved owner.
	Enroll EnrollFn
	// Leave commits authenticated voluntary departure.
	Leave LeaveFn
	// Queue holds join requests; nil refuses redemptions that need approval.
	Queue QueueFn
	// Withdraw removes join requests; nil holds none to remove.
	Withdraw WithdrawFn
	// HeldConfig reads the Spaces a participant_of invite names; nil holds none.
	HeldConfig HeldConfigFn
}

// Server implements the SOInviteService SRPC server.
type Server struct {
	// le provides diagnostics for this service.
	le *logrus.Entry
	// h performs the owner operations.
	h Handlers
	// acceptMtx serializes invite acceptance from lookup through the use
	// increment, so a limited-use invite cannot enroll more peers than it admits.
	acceptMtx csync.Mutex
}

// NewServer constructs a new SO invite server.
func NewServer(le *logrus.Entry, h Handlers) *Server {
	return &Server{le: le, h: h}
}

// Leave binds the transport to a departing identity before invoking the native owner.
func (s *Server) Leave(ctx context.Context, request *sobject.SOLeaveRequest) (*sobject.SOLeaveResponse, error) {
	// A valid proof from another connection cannot be relayed as fresh caller authority.
	peers, err := request.Verify()
	if err != nil {
		return nil, err
	}
	stream := link.GetMountedStreamContext(ctx)
	if stream == nil || !slices.Contains(peers, stream.GetPeerID().String()) {
		return nil, errors.New("leave stream does not match a departing identity")
	}

	// The mounted provider retains and mutates its own host for the complete operation.
	if s.h.Leave == nil {
		return nil, errors.New("voluntary departure is unavailable")
	}
	return s.h.Leave(ctx, request)
}

// WithdrawJoinRequest removes the stream peer's pending join request.
func (s *Server) WithdrawJoinRequest(ctx context.Context, req *WithdrawJoinRequestRequest) (*WithdrawJoinRequestResponse, error) {
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
func (s *Server) AcceptInvite(ctx context.Context, req *AcceptInviteRequest) (*AcceptInviteResponse, error) {
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
	// The invitee proves possession of the raw token; the on-chain state
	// stores only the SHA256 hash.
	tokenHashArr := sha256.Sum256(token)
	tokenHash := tokenHashArr[:]

	// Verify the invitee is who they say they are via mounted stream context.
	ms := link.GetMountedStreamContext(ctx)
	if ms == nil {
		return nil, errors.New("no mounted stream context")
	}
	streamPeerID := ms.GetPeerID()

	// Parse the responder peer ID from the join response.
	responderPeerID, responderPubKey, err := ValidateJoinResponse(joinResp)
	if err != nil {
		return nil, err
	}

	// The stream peer must match the join response author.
	if streamPeerID != responderPeerID {
		return nil, errors.New("stream peer ID does not match join response responder")
	}

	// The storage join response must be valid and name the same invite.
	storageJoinResp := req.GetStorageJoinResponse()
	if storageJoinResp == nil {
		return nil, errors.New("storage_join_response is required")
	}
	storagePeerID, storagePubKey, err := ValidateJoinResponse(storageJoinResp)
	if err != nil {
		return nil, errors.Wrap(err, "validate storage join response")
	}
	if storageJoinResp.GetInviteId() != joinResp.GetInviteId() {
		return nil, errors.New("storage join response invite ID mismatch")
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

	// Validate the responder may redeem the invite: it is the named peer of a
	// targeted invite, or the invite is untargeted, and it is not revoked,
	// expired or maxed.
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
		if err := s.h.Queue(ctx, result, joinResp); err != nil {
			return nil, errors.Wrap(err, "queue join request")
		}
		return &AcceptInviteResponse{SharedObjectId: result.SharedObjectID, Pending: true}, nil
	}

	// Enroll the participant first. If enrollment fails, the invite use
	// is not consumed (avoids burning limited-use invites on transient errors).
	if s.h.Enroll == nil {
		return nil, errors.New("enrollment not configured")
	}
	grant, err := s.h.Enroll(ctx, result, responderPeerID, responderPubKey)
	if err != nil {
		return nil, errors.Wrap(err, "enroll participant")
	}
	if storagePeerID != responderPeerID {
		if _, err := s.h.Enroll(ctx, result, storagePeerID, storagePubKey); err != nil {
			return nil, errors.Wrap(err, "enroll storage participant")
		}
	}

	// Enrollment succeeded. Increment invite uses, unless the named peer of a
	// targeted invite is redeeming it again.
	if sobject.InviteCountsUse(result.Invite) {
		inviteMutator := result.InviteMutator
		if inviteMutator == nil {
			inviteMutator = result.Host
		}
		if err := inviteMutator.IncrementInviteUses(ctx, result.OwnerPrivKey, result.Invite.GetInviteId()); err != nil {
			return nil, errors.Wrap(err, "increment invite uses")
		}
	}

	// Transfer the configuration after every acceptance mutation has completed.
	ownerState, err := result.Host.GetHostState(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "read updated owner shared object state")
	}
	if err := result.Host.WaitDurable(ctx); err != nil {
		return nil, errors.Wrap(err, "wait for durable shared object state")
	}

	// Return the grant with the owner state and its config lineage.
	lineage, err := result.Host.ReadConfigLineage(ctx, ownerState.GetConfig().GetConfigChainHash())
	if err != nil {
		return nil, errors.Wrap(err, "read shared object config lineage")
	}
	return &AcceptInviteResponse{
		Grant:             grant,
		SharedObjectId:    result.SharedObjectID,
		SharedObjectState: ownerState.CloneVT(),
		ConfigLineage:     lineage,
	}, nil
}

// readHeldConfigs reads this host's configuration of each Space the invite names.
func (s *Server) readHeldConfigs(ctx context.Context, inv *sobject.SOInvite) (map[string]*sobject.SharedObjectConfig, error) {
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
var _ SRPCSOInviteServiceServer = (*Server)(nil)
