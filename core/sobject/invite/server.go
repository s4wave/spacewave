package sobject_invite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"slices"

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

// Server implements the SOInviteService SRPC server.
type Server struct {
	// le provides diagnostics for this service.
	le *logrus.Entry
	// lookupFn resolves the owner's invitation authority.
	lookupFn InviteLookupFn
	// enrollFn issues participant grants under the resolved owner.
	enrollFn EnrollFn
	// leaveFn commits authenticated voluntary departure.
	leaveFn LeaveFn
}

// NewServer constructs a new SO invite server.
func NewServer(le *logrus.Entry, lookupFn InviteLookupFn, enrollFn EnrollFn, leaveFn LeaveFn) *Server {
	return &Server{
		le:       le,
		lookupFn: lookupFn,
		enrollFn: enrollFn,
		leaveFn:  leaveFn,
	}
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
	if s.leaveFn == nil {
		return nil, errors.New("voluntary departure is unavailable")
	}
	return s.leaveFn(ctx, request)
}

// AcceptInvite processes a join request from an invitee.
func (s *Server) AcceptInvite(ctx context.Context, req *AcceptInviteRequest) (*AcceptInviteResponse, error) {
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

	// Look up the invite by token hash.
	result, err := s.lookupFn(ctx, tokenHash)
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

	// Check target_peer_id constraint if set.
	if targetPeer := result.Invite.GetTargetPeerId(); targetPeer != "" {
		if responderPeerID.String() != targetPeer {
			return nil, errors.New("invite is targeted to a different peer")
		}
	}

	// Validate the invite is still usable (not revoked, not expired, not maxed).
	if err := sobject.ValidateInviteUsable(result.Invite); err != nil {
		return nil, errors.Wrap(err, "invite not usable")
	}

	// Enroll the participant first. If enrollment fails, the invite use
	// is not consumed (avoids burning limited-use invites on transient errors).
	if s.enrollFn == nil {
		return nil, errors.New("enrollment not configured")
	}
	grant, err := s.enrollFn(ctx, result, responderPeerID, responderPubKey)
	if err != nil {
		return nil, errors.Wrap(err, "enroll participant")
	}
	if storagePeerID != responderPeerID {
		if _, err := s.enrollFn(ctx, result, storagePeerID, storagePubKey); err != nil {
			return nil, errors.Wrap(err, "enroll storage participant")
		}
	}

	// Preserve the owner's root grant on the joined copy. Without it, state
	// written by the invitee can no longer be decoded by the originating owner.
	ownerPeerID, err := peer.IDFromPrivateKey(result.OwnerPrivKey)
	if err != nil {
		return nil, errors.Wrap(err, "derive owner peer ID")
	}
	ownerState, err := result.Host.GetHostState(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "read owner shared object state")
	}
	var ownerGrant *sobject.SOGrant
	for _, candidate := range ownerState.GetRootGrants() {
		if candidate.GetPeerId() == ownerPeerID.String() {
			ownerGrant = candidate.CloneVT()
			break
		}
	}
	if ownerGrant == nil {
		return nil, errors.New("owner root grant not found")
	}

	// Enrollment succeeded. Increment invite uses.
	inviteMutator := result.InviteMutator
	if inviteMutator == nil {
		inviteMutator = result.Host
	}
	if err := inviteMutator.IncrementInviteUses(ctx, result.OwnerPrivKey, result.Invite.GetInviteId()); err != nil {
		return nil, errors.Wrap(err, "increment invite uses")
	}

	// Transfer the configuration after every acceptance mutation has completed.
	ownerState, err = result.Host.GetHostState(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "read updated owner shared object state")
	}
	if err := result.Host.WaitDurable(ctx); err != nil {
		return nil, errors.Wrap(err, "wait for durable shared object state")
	}

	return &AcceptInviteResponse{
		Grant:             grant,
		SharedObjectId:    result.SharedObjectID,
		OwnerGrant:        ownerGrant,
		SharedObjectState: ownerState.CloneVT(),
	}, nil
}

// _ is a type assertion.
var _ SRPCSOInviteServiceServer = (*Server)(nil)
