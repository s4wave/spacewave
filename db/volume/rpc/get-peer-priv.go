package volume_rpc

import (
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// NewGetPeerPrivResponse builds a new GetPeerPriv response object.
func NewGetPeerPrivResponse(privKey crypto.PrivKey) (*GetPeerPrivResponse, error) {
	privKeyTxt, err := confparse.MarshalPrivateKey(privKey)
	if err != nil {
		return nil, err
	}
	return &GetPeerPrivResponse{PrivKey: privKeyTxt}, nil
}

// ParsePeer constructs the volume peer from the response's required private key.
func (r *GetPeerPrivResponse) ParsePeer() (peer.Peer, error) {
	// Decode the response and require the private key promised by this RPC.
	privKey, err := confparse.ParsePrivateKey(r.GetPrivKey())
	if err != nil {
		return nil, err
	}
	if privKey == nil {
		return nil, peer.ErrNoPrivKey
	}

	// Construct the peer only after its private key is available.
	return peer.NewPeer(privKey)
}
