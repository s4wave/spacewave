package rpc_volume

import (
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
	"github.com/libp2p/go-libp2p/core/crypto"
)

// NewGetPeerPrivResponse builds a new GetPeerPriv response object.
func NewGetPeerPrivResponse(privKey crypto.PrivKey) (*GetPeerPrivResponse, error) {
	privKeyTxt, err := confparse.MarshalPrivateKey(privKey)
	if err != nil {
		return nil, err
	}
	return &GetPeerPrivResponse{PrivKey: privKeyTxt}, nil
}

// Validate validates the response object.
func (r *GetPeerPrivResponse) Validate() error {
	privKey, err := r.ParsePrivKey()
	if err == nil && privKey == nil {
		return peer.ErrNoPrivKey
	}
	return err
}

// ParsePrivKey parses the private key field.
// Returns nil, nil if the response field was empty.
func (r *GetPeerPrivResponse) ParsePrivKey() (crypto.PrivKey, error) {
	return confparse.ParsePrivateKey(r.GetPrivKey())
}
