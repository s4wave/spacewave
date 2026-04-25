package sobject_invite

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// ValidateJoinResponse validates a signed join response and returns the
// responder identity.
func ValidateJoinResponse(
	joinResp *sobject.SOJoinResponse,
) (peer.ID, crypto.PubKey, error) {
	if joinResp == nil {
		return "", nil, errors.New("join_response is required")
	}

	responderPeerID, err := peer.IDB58Decode(joinResp.GetResponderPeerId())
	if err != nil {
		return "", nil, errors.Wrap(err, "parse responder peer ID")
	}

	responderPubKey, err := crypto.UnmarshalPublicKey(joinResp.GetResponderPubkey())
	if err != nil {
		return "", nil, errors.Wrap(err, "parse responder public key")
	}

	derivedPeerID, err := peer.IDFromPublicKey(responderPubKey)
	if err != nil {
		return "", nil, errors.Wrap(err, "derive peer ID from public key")
	}
	if derivedPeerID != responderPeerID {
		return "", nil, errors.New("responder public key does not match responder peer ID")
	}

	sig := joinResp.GetSignature()
	if sig == nil {
		return "", nil, errors.New("join response signature is required")
	}

	signData, err := (&sobject.SOJoinResponse{
		InviteId:        joinResp.GetInviteId(),
		ResponderPeerId: joinResp.GetResponderPeerId(),
		ResponderPubkey: joinResp.GetResponderPubkey(),
	}).MarshalVT()
	if err != nil {
		return "", nil, errors.Wrap(err, "marshal join response for verification")
	}

	valid, err := sig.VerifyWithPublic("sobject join response", responderPubKey, signData)
	if err != nil {
		return "", nil, errors.Wrap(err, "verify join response signature")
	}
	if !valid {
		return "", nil, errors.New("join response signature is invalid")
	}

	return responderPeerID, responderPubKey, nil
}
