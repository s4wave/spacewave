package session

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// ExtractPublicKeyFromPeerID decodes a base58 peer ID and extracts its public key.
func ExtractPublicKeyFromPeerID(peerID string) (crypto.PubKey, error) {
	// Decode the peer identity before extracting its public key.
	pid, err := peer.IDB58Decode(peerID)
	if err != nil {
		return nil, errors.Wrap(err, "decode peer ID")
	}

	// Extract the public key embedded in the decoded peer identity.
	pub, err := pid.ExtractPublicKey()
	if err != nil {
		return nil, errors.Wrap(err, "extract public key")
	}
	return pub, nil
}
