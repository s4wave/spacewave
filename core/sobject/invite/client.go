package sobject_invite

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	stream_srpc "github.com/s4wave/spacewave/net/stream/srpc"
	"github.com/zeebo/blake3"

	"github.com/s4wave/spacewave/core/sobject"
)

// JoinResult contains the result of a successful invite join.
type JoinResult struct {
	// Grant is the encrypted SOGrant for the invitee.
	Grant *sobject.SOGrant
	// SharedObjectID is the ID of the shared object.
	SharedObjectID string
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
	inviteMsg *sobject.SOInviteMessage,
) (*JoinResult, error) {
	if inviteMsg == nil {
		return nil, errors.New("invite message is nil")
	}

	ownerPeerID, err := peer.IDB58Decode(inviteMsg.GetOwnerPeerId())
	if err != nil {
		return nil, errors.Wrap(err, "parse owner peer ID from invite")
	}

	// Build the signed join response.
	joinResp, err := BuildJoinResponse(inviteMsg.GetInviteId(), inviteePrivKey)
	if err != nil {
		return nil, errors.Wrap(err, "build join response")
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
		JoinResponse: joinResp,
		Token:        inviteMsg.GetToken(),
	})
	if err != nil {
		return nil, errors.Wrap(err, "accept invite RPC")
	}

	return &JoinResult{
		Grant:          resp.GetGrant(),
		SharedObjectID: resp.GetSharedObjectId(),
	}, nil
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

	sig, err := peer.NewSignature("sobject join response", privKey, hash.HashType_HashType_BLAKE3, signData, true)
	if err != nil {
		return nil, errors.Wrap(err, "sign join response")
	}

	unsigned.Signature = sig
	return unsigned, nil
}

// HashInviteToken computes the BLAKE3 hash of a raw invite token.
// Used by the invitee to produce the token_hash for AcceptInviteRequest.
func HashInviteToken(token []byte) []byte {
	h := blake3.Sum256(token)
	return h[:]
}
