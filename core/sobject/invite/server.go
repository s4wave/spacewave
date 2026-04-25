package sobject_invite

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	stream_srpc_server "github.com/s4wave/spacewave/net/stream/srpc/server"
	"github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"

	"github.com/s4wave/spacewave/core/sobject"
)

// ProtocolID is the bifrost protocol ID for the SO invite handshake.
const ProtocolID = protocol.ID("alpha/so-invite")

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "alpha/so-invite/server"

// InviteLookupResult contains the resolved invite and its context.
type InviteLookupResult struct {
	// Host is the SOHost managing the shared object.
	Host *sobject.SOHost
	// InviteMutator applies invite state mutations for this shared object.
	InviteMutator sobject.InviteMutator
	// Invite is the matching SOInvite.
	Invite *sobject.SOInvite
	// SharedObjectID is the ID of the shared object.
	SharedObjectID string
	// OwnerPrivKey is the owner's private key for signing config changes.
	OwnerPrivKey crypto.PrivKey
}

// InviteLookupFn resolves an invite by token hash.
// Returns nil result if no matching invite is found.
type InviteLookupFn func(ctx context.Context, tokenHash []byte) (*InviteLookupResult, error)

// EnrollFn enrolls a participant after invite verification.
// Called with the resolved invite context and the invitee's identity.
// Returns the SOGrant for the invitee.
type EnrollFn func(ctx context.Context, result *InviteLookupResult, inviteePeerID peer.ID, inviteePubKey crypto.PubKey) (*sobject.SOGrant, error)

// Server implements the SOInviteService SRPC server.
type Server struct {
	le       *logrus.Entry
	lookupFn InviteLookupFn
	enrollFn EnrollFn
}

// NewServer constructs a new SO invite server.
func NewServer(le *logrus.Entry, lookupFn InviteLookupFn, enrollFn EnrollFn) *Server {
	return &Server{
		le:       le,
		lookupFn: lookupFn,
		enrollFn: enrollFn,
	}
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
	// stores only the BLAKE3 hash.
	tokenHashArr := blake3.Sum256(token)
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

	// Enrollment succeeded. Increment invite uses.
	inviteMutator := result.InviteMutator
	if inviteMutator == nil {
		inviteMutator = result.Host
	}
	if err := inviteMutator.IncrementInviteUses(ctx, result.OwnerPrivKey, result.Invite.GetInviteId()); err != nil {
		return nil, errors.Wrap(err, "increment invite uses")
	}

	return &AcceptInviteResponse{
		Grant:          grant,
		SharedObjectId: result.SharedObjectID,
	}, nil
}

// InviteController wraps the SRPC server with the bifrost stream handler.
type InviteController struct {
	*stream_srpc_server.Server
	srv *Server
}

// NewInviteController constructs an SO invite controller.
func NewInviteController(
	le *logrus.Entry,
	b bus.Bus,
	lookupFn InviteLookupFn,
	enrollFn EnrollFn,
	peerIDs []string,
) (*InviteController, error) {
	srv := NewServer(le, lookupFn, enrollFn)
	ctrl := &InviteController{srv: srv}
	var err error
	ctrl.Server, err = stream_srpc_server.NewServer(
		b,
		le,
		controller.NewInfo(ControllerID, Version, "so invite server"),
		[]stream_srpc_server.RegisterFn{
			func(mux srpc.Mux) error {
				return SRPCRegisterSOInviteService(mux, srv)
			},
		},
		[]protocol.ID{ProtocolID},
		peerIDs,
		false,
	)
	if err != nil {
		return nil, err
	}
	return ctrl, nil
}

// _ is a type assertion
var _ SRPCSOInviteServiceServer = ((*Server)(nil))
