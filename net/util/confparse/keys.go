package confparse

import (
	"strings"

	b58 "github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// ParsePeer parses one of privKey, pubKey, peerID to build a Peer.
func ParsePeer(
	// PrivKey is the peer private key in either b58 or PEM format.
	// See confparse.MarshalPrivateKey.
	// If not set, the peer private key will be unavailable.
	privKey string,
	// PubKey is the peer public key.
	// Ignored if priv_key is set.
	pubKey string,
	// PeerId is the peer identifier.
	// Ignored if priv_key or pub_key are set.
	// The peer ID should contain the public key.
	peerId string,
) (peer.Peer, error) {
	pkey, err := ParsePrivateKey(privKey)
	if err != nil {
		return nil, err
	}
	if pkey != nil {
		return peer.NewPeer(pkey)
	}

	pub, err := ParsePublicKey(pubKey)
	if err != nil {
		return nil, err
	}
	if pub != nil {
		return peer.NewPeerWithPubKey(pub)
	}

	peerID, err := ParsePeerID(peerId)
	if err != nil {
		return nil, err
	}
	if peerID == "" {
		return nil, errors.New("one of priv_key, pub_key, peer_id must be set")
	}

	return peer.NewPeerWithID(peerID)
}

// ParsePublicKey parses the public key from a string.
// If the string starts with "-----BEGIN" assumes it is PEM.
// Otherwise: the string is a b58 encoded libp2p public key.
// If there is no public key specified, returns nil, nil.
func ParsePublicKey(pubKeyStr string) (crypto.PubKey, error) {
	pubKeyStr = strings.TrimSpace(pubKeyStr)
	if len(pubKeyStr) == 0 {
		return nil, nil
	}

	if strings.HasPrefix(pubKeyStr, "-----BEGIN") {
		return ParsePublicKeyPEM([]byte(pubKeyStr))
	}

	data, err := b58.Decode(pubKeyStr)
	if err != nil {
		return nil, errors.New("public key must be valid b58")
	}

	return crypto.UnmarshalPublicKey(data)
}

// MarshalPublicKey marshals the public key in b58 format.
func MarshalPublicKey(key crypto.PubKey) (string, error) {
	if key == nil {
		return "", nil
	}

	data, err := crypto.MarshalPublicKey(key)
	if err != nil {
		return "", err
	}
	return b58.Encode(data), nil
}

// ParsePrivateKey parses the private key from a string.
// If the string starts with "-----BEGIN" assumes it is PEM.
// Otherwise: the string is a b58 encoded libp2p public key.
// If there is no public key specified, returns nil, nil.
func ParsePrivateKey(privKeyStr string) (crypto.PrivKey, error) {
	privKeyStr = strings.TrimSpace(privKeyStr)
	if len(privKeyStr) == 0 {
		return nil, nil
	}

	if strings.HasPrefix(privKeyStr, "-----BEGIN") {
		return ParsePrivateKeyPEM([]byte(privKeyStr))
	}

	data, err := b58.Decode(privKeyStr)
	if err != nil {
		return nil, errors.New("private key must be valid b58")
	}
	return crypto.UnmarshalPrivateKey(data)
}

// MarshalPrivateKey marshals the private key in b58 format.
func MarshalPrivateKey(key crypto.PrivKey) (string, error) {
	if key == nil {
		return "", nil
	}

	data, err := crypto.MarshalPrivateKey(key)
	if err != nil {
		return "", err
	}
	return b58.Encode(data), nil
}

// ValidatePubKey checks if a public key is set and valid.
//
// if the peer id is given, checks if it matches
func ValidatePubKey(id string, peerID peer.ID) error {
	pkey, err := ParsePublicKey(id)
	if err == nil && pkey == nil {
		err = errors.New("pub_key cannot be empty")
	}
	if err != nil || len(peerID) == 0 {
		return err
	}

	if !peerID.MatchesPublicKey(pkey) {
		pkeyID, err := peer.IDFromPublicKey(pkey)
		if err != nil {
			return err
		}

		return errors.Errorf(
			"pub_key id %s does not match peer_id %s",
			pkeyID.String(),
			peerID.String(),
		)
	}

	return nil
}
