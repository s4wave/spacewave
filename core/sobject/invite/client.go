package sobject_invite

import (
	"context"
	"crypto/sha256"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	stream_srpc "github.com/s4wave/spacewave/net/stream/srpc"
)

// JoinResult contains the result of an accepted invite redemption.
type JoinResult struct {
	// Grant is the encrypted SOGrant for the invitee.
	Grant *sobject.SOGrant
	// SharedObjectID is the ID of the shared object.
	SharedObjectID string
	// SharedObjectState is the owner's authorized state after enrollment.
	SharedObjectState *sobject.SOState
	// ConfigLineage is the owner's retained configuration changes leading to
	// the config of SharedObjectState, oldest first.
	ConfigLineage []*sobject.SOConfigChange
	// Pending is set when the owner queued the redemption for approval
	// instead of enrolling the invitee.
	Pending bool
}

// JoinViaInvite executes the invitee side of the invite handshake.
//
// The invitee must already have a session transport running with a child bus
// that can reach the owner's peer (via signaling/WebRTC). This function:
// 1. Builds and signs a SOJoinResponse
// 2. Opens an SRPC stream to the owner on protocol alpha/so-invite
// 3. Sends AcceptInviteRequest with the raw token
// 4. Returns the SOGrant from the owner
func JoinViaInvite(
	ctx context.Context,
	childBus bus.Bus,
	localPeerID peer.ID,
	inviteePrivKey crypto.PrivKey,
	storagePrivKey crypto.PrivKey,
	inviteMsg *sobject.SOInviteMessage,
) (*JoinResult, error) {
	// Verify the owner peer the invite routes to.
	if inviteMsg == nil {
		return nil, errors.New("invite message is nil")
	}
	ownerPeerID, err := inviteMsg.VerifyTransportPeer()
	if err != nil {
		return nil, errors.Wrap(err, "parse owner peer ID from invite")
	}

	// Build the signed join responses for the session and storage peers.
	joinResp, err := BuildJoinResponse(inviteMsg.GetInviteId(), inviteePrivKey)
	if err != nil {
		return nil, errors.Wrap(err, "build join response")
	}
	storageJoinResp, err := BuildJoinResponse(inviteMsg.GetInviteId(), storagePrivKey)
	if err != nil {
		return nil, errors.Wrap(err, "build storage join response")
	}

	// Create SRPC client targeting the owner on alpha/so-invite protocol.
	openFn := stream_srpc.NewOpenStreamFunc(
		childBus,
		ProtocolID,
		localPeerID,
		ownerPeerID,
		0,
	)
	client := NewSRPCSOInviteServiceClient(srpc.NewClient(openFn))

	// Send the AcceptInvite request with the raw token.
	resp, err := client.AcceptInvite(ctx, &AcceptInviteRequest{
		JoinResponse:        joinResp,
		Token:               inviteMsg.GetToken(),
		StorageJoinResponse: storageJoinResp,
	})
	if err != nil {
		return nil, errors.Wrap(err, "accept invite RPC")
	}

	return &JoinResult{
		Grant:             resp.GetGrant(),
		SharedObjectID:    resp.GetSharedObjectId(),
		SharedObjectState: resp.GetSharedObjectState(),
		ConfigLineage:     resp.GetConfigLineage(),
		Pending:           resp.GetPending(),
	}, nil
}

// LeaveSharedObject sends signed departure consent to the authenticated native owner.
// The caller verifies the returned configuration proof before retiring its local access.
func LeaveSharedObject(ctx context.Context, childBus bus.Bus, localPeerID, ownerPeerID peer.ID, request *sobject.SOLeaveRequest) (*sobject.SOLeaveResponse, error) {
	open := stream_srpc.NewOpenStreamFunc(childBus, ProtocolID, localPeerID, ownerPeerID, 0)
	return NewSRPCSOInviteServiceClient(srpc.NewClient(open)).Leave(ctx, request)
}

// WithdrawJoinRequest asks the owner to remove the caller's pending request to
// join the shared object.
func WithdrawJoinRequest(ctx context.Context, childBus bus.Bus, localPeerID, ownerPeerID peer.ID, sharedObjectID string) error {
	open := stream_srpc.NewOpenStreamFunc(childBus, ProtocolID, localPeerID, ownerPeerID, 0)
	_, err := NewSRPCSOInviteServiceClient(srpc.NewClient(open)).WithdrawJoinRequest(ctx, &WithdrawJoinRequestRequest{
		SharedObjectId: sharedObjectID,
	})
	return err
}

// BuildJoinResponse constructs and signs a SOJoinResponse for an invite.
// The invitee calls this with their private key and the invite details.
func BuildJoinResponse(inviteID string, privKey crypto.PrivKey) (*sobject.SOJoinResponse, error) {
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, errors.Wrap(err, "derive peer ID")
	}

	pubKey := privKey.GetPublic()
	pubKeyBytes, err := crypto.MarshalPublicKey(pubKey)
	if err != nil {
		return nil, errors.Wrap(err, "marshal public key")
	}

	// Build the unsigned message for signing.
	unsigned := &sobject.SOJoinResponse{
		InviteId:        inviteID,
		ResponderPeerId: peerID.String(),
		ResponderPubkey: pubKeyBytes,
	}
	signData, err := unsigned.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal join response for signing")
	}

	sig, err := peer.NewSignature("sobject join response", privKey, hash.RecommendedHashType, signData, true)
	if err != nil {
		return nil, errors.Wrap(err, "sign join response")
	}

	unsigned.Signature = sig
	return unsigned, nil
}

// HashInviteToken computes the SHA256 hash of a raw invite token.
// Used by clients that need to compare raw invite tokens with SOInvite.token_hash.
func HashInviteToken(token []byte) []byte {
	h := sha256.Sum256(token)
	return h[:]
}
