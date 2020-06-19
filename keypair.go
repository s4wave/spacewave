package identity

import (
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/libp2p/go-libp2p-core/crypto"
)

// NewKeypair constructs a new keypair.
func NewKeypair(pubKey crypto.PubKey) (*Keypair, error) {
	pid, err := peer.IDFromPublicKey(pubKey)
	if err != nil {
		return nil, err
	}
	pkData, err := crypto.MarshalPublicKey(pubKey)
	if err != nil {
		return nil, err
	}
	return &Keypair{
		PeerId:     pid.Pretty(),
		PubKeyData: pkData,
		// auth provider empty
	}, nil
}
