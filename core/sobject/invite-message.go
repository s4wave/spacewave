package sobject

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// Sign signs every invitation field with the declared owner's key.
func (m *SOInviteMessage) Sign(ownerKey crypto.PrivKey) error {
	// Require the signing key to be the declared owner's key.
	ownerID, err := peer.IDFromPrivateKey(ownerKey)
	if err != nil {
		return err
	}
	if ownerID.String() != m.GetOwnerPeerId() {
		return errors.New("invitation signing key does not match owner")
	}

	// Exclude only the signature itself from the signed message.
	unsigned := m.CloneVT()
	unsigned.Signature = nil
	data, err := unsigned.MarshalVT()
	if err != nil {
		return err
	}
	signature, err := peer.NewSignature("sobject invite", ownerKey, hash.RecommendedHashType, data, true)
	if err != nil {
		return err
	}
	m.Signature = signature
	return nil
}

// VerifyTransportPeer verifies the owner's signature before resolving the peer
// that receives the invitation token. Older invitations dial the owner directly.
func (m *SOInviteMessage) VerifyTransportPeer() (peer.ID, error) {
	// Require a signature and encode the body it covers.
	if m == nil || m.GetSignature() == nil {
		return "", errors.New("invitation signature is required")
	}
	unsigned := m.CloneVT()
	unsigned.Signature = nil
	data, err := unsigned.MarshalVT()
	if err != nil {
		return "", err
	}

	// Authenticate the owner's endpoint before disclosing the one-use token to it.
	ownerID, err := verifySignature(m.GetSignature(), "sobject invite", data)
	if err != nil {
		return "", errors.Wrap(err, "invitation signature")
	}
	if ownerID.String() != m.GetOwnerPeerId() {
		return "", errors.New("invitation signature does not match owner")
	}

	// Prefer the invitation's transport peer and fall back to the owner.
	if endpoint := m.GetTransportPeerId(); endpoint != "" {
		return peer.IDB58Decode(endpoint)
	}
	return ownerID, nil
}
