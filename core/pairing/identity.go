package pairing

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_invite "github.com/s4wave/spacewave/core/sobject/invite"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

// BuildIdentity binds the receiving Session and storage keys to one offered
// account, Session reference, and authenticated transport connection.
func BuildIdentity(offer *AccountOffer, ref *session.SessionRef, sessionKey, storageKey crypto.PrivKey, sourcePeer, receivingPeer peer.ID) (*Identity, error) {
	// Build the identity context and sign both receiving keys over it.
	proofContext, err := IdentityContext(offer, ref, sourcePeer, receivingPeer)
	if err != nil {
		return nil, err
	}
	sessionProof, err := sobject_invite.BuildJoinResponse(proofContext, sessionKey)
	if err != nil {
		return nil, err
	}
	storageProof, err := sobject_invite.BuildJoinResponse(proofContext, storageKey)
	if err != nil {
		return nil, err
	}
	return &Identity{SessionRef: ref, SessionProof: sessionProof, StorageProof: storageProof}, nil
}

// IdentityContext prevents a proof from authorizing another account, receiving
// reference, transport connection, or pairing attempt.
func IdentityContext(offer *AccountOffer, ref *session.SessionRef, sourcePeer, receivingPeer peer.ID) (string, error) {
	// Reject an offer or receiving reference that does not attach to the account.
	if offer.GetAccountId() == "" || offer.GetOperationId() == "" {
		return "", errors.New("pairing account identity is incomplete")
	}
	if err := ref.Validate(); err != nil {
		return "", err
	}
	if ref.GetProviderResourceRef().GetProviderAccountId() != offer.GetAccountId() || ref.GetProviderResourceRef().GetProviderId() != SessionProviderID(offer) {
		return "", errors.New("receiving Session does not attach to the offered account")
	}
	if sourcePeer == "" || receivingPeer == "" || sourcePeer == receivingPeer {
		return "", errors.New("pairing requires distinct authenticated transport peers")
	}

	// Digest the offer, receiving reference, and both transport peers.
	data, err := (&Frame{Body: &Frame_Account{Account: offer}}).MarshalVT()
	if err != nil {
		return "", err
	}

	// Marshal the receiving reference the context must bind.
	identity, err := (&Identity{SessionRef: ref}).MarshalVT()
	if err != nil {
		return "", err
	}

	// Digest the offer, receiving reference, and both transport peers.
	digest := sha256.New()
	digest.Write(data)
	digest.Write(identity)
	digest.Write([]byte(sourcePeer.String() + "/" + receivingPeer.String()))
	return "account pairing/" + hex.EncodeToString(digest.Sum(nil)), nil
}

// ValidateIdentity verifies both receiving key proofs before authorization.
func ValidateIdentity(offer *AccountOffer, identity *Identity, sourcePeer, receivingPeer peer.ID) error {
	proofContext, err := IdentityContext(offer, identity.GetSessionRef(), sourcePeer, receivingPeer)
	if err != nil {
		return err
	}
	for _, proof := range []*sobject.SOJoinResponse{identity.GetSessionProof(), identity.GetStorageProof()} {
		if proof.GetInviteId() != proofContext {
			return errors.New("pairing identity proof does not match the approved operation")
		}
		if _, _, err := sobject_invite.ValidateJoinResponse(proof); err != nil {
			return err
		}
	}
	return nil
}

// ApprovalContext binds bilateral approval to both complete signed proofs.
func ApprovalContext(offer *AccountOffer, identity *Identity, sourcePeer, receivingPeer peer.ID) (string, error) {
	// Digest the proof context with the complete signed identity.
	proofContext, err := IdentityContext(offer, identity.GetSessionRef(), sourcePeer, receivingPeer)
	if err != nil {
		return "", err
	}
	data, err := identity.MarshalVT()
	if err != nil {
		return "", err
	}
	digest := sha256.New()
	digest.Write([]byte(proofContext))
	digest.Write(data)
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// ReceiveFrame returns the next enrollment frame or its terminal error.
func ReceiveFrame(stream *stream_packet.Session) (*Frame, error) {
	frame := &Frame{}
	if err := stream.RecvMsg(frame); err != nil {
		return nil, err
	}
	if message := frame.GetError(); message != "" {
		return nil, errors.New(message)
	}
	return frame, nil
}
