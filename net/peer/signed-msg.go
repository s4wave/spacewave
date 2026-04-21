package peer

import (
	"bytes"
	"encoding/hex"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/zeebo/blake3"
)

// NewSignedMsg constructs/signs/encodes a new signed message.
//
// encContext strings must be hardcoded constants, and the recommended
// format is "[application] [commit timestamp] [purpose]", e.g.,
// "example.com 2019-12-25 16:18:03 session tokens v1".
func NewSignedMsg(
	encContext string,
	privKey crypto.PrivKey,
	hashType hash.HashType,
	innerData []byte,
) (*SignedMsg, error) {
	peerID, err := IDFromPrivateKey(privKey)
	if err != nil {
		return nil, err
	}
	msg := &SignedMsg{
		FromPeerId: IDB58Encode(peerID),
		Data:       innerData,
	}
	if err := msg.Sign(encContext, privKey, hashType); err != nil {
		return nil, err
	}
	return msg, nil
}

// UnmarshalSignedMsg parses a signed message.
func UnmarshalSignedMsg(data []byte) (*SignedMsg, error) {
	m := &SignedMsg{}
	err := m.UnmarshalVT(data)
	if err != nil {
		return nil, err
	}
	return m, err
}

// ComputeMessageID computes a message id for a signed message.
func (m *SignedMsg) ComputeMessageID() string {
	inner := bytes.Join([][]byte{
		m.GetSignature().GetSigData(),
		[]byte(m.GetFromPeerId()),
	}, nil)
	h := blake3.Sum256(inner)
	return hex.EncodeToString(h[:])
}

// ParseFromPeerID unmarshals the peer id.
func (m *SignedMsg) ParseFromPeerID() (ID, error) {
	return IDB58Decode(m.GetFromPeerId())
}

// ExtractPubKey extracts the public key from the peer id.
func (m *SignedMsg) ExtractPubKey() (crypto.PubKey, ID, error) {
	fromPeerID, err := m.ParseFromPeerID()
	if err != nil {
		return nil, ID(""), err
	}
	if err := fromPeerID.Validate(); err != nil {
		return nil, ID(""), errors.Wrap(err, "message peer id")
	}
	pubKey, err := fromPeerID.ExtractPublicKey()
	if err != nil {
		return nil, fromPeerID, err
	}
	return pubKey, fromPeerID, nil
}

// ExtractAndVerify extracts public key & uses it to verify message
//
// encContext must match the context used when creating the signature.
func (m *SignedMsg) ExtractAndVerify(encContext string) (crypto.PubKey, ID, error) {
	if len(m.GetData()) == 0 {
		return nil, "", ErrEmptyBody
	}
	if len(m.GetFromPeerId()) == 0 {
		return nil, "", ErrEmptyPeerID
	}
	if err := m.GetSignature().Validate(); err != nil {
		return nil, "", errors.Wrap(err, "message signature")
	}

	pubKey, peerID, err := m.ExtractPubKey()
	if err != nil {
		return nil, peerID, err
	}

	sigErr := m.Verify(encContext, pubKey)
	if sigErr != nil {
		return pubKey, peerID, err
	}

	return pubKey, peerID, nil
}

// Sign signs the inner body with the private key.
// Disallows empty message.
//
// encContext strings must be hardcoded constants, and the recommended
// format is "[application] [commit timestamp] [purpose]", e.g.,
// "example.com 2019-12-25 16:18:03 session tokens v1".
func (m *SignedMsg) Sign(encContext string, privKey crypto.PrivKey, hashType hash.HashType) error {
	innerData := m.GetData()
	if len(innerData) == 0 {
		return ErrEmptyBody
	}

	sig, err := NewSignature(
		encContext,
		privKey,
		hashType,
		innerData,
		false,
	)
	if err != nil {
		return err
	}
	m.Signature = sig
	return nil
}

// Verify verifies the signature against a public key.
//
// encContext must match the context used when creating the signature.
func (m *SignedMsg) Verify(encContext string, pubKey crypto.PubKey) error {
	// validate signature
	sigOk, sigErr := m.GetSignature().VerifyWithPublic(encContext, pubKey, m.GetData())
	if !sigOk && sigErr == nil {
		sigErr = ErrSignatureInvalid
	}
	return sigErr
}
