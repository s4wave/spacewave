package peer_api

import (
	"github.com/s4wave/spacewave/net/peer"
)

// NewPeerInfo builds peer info from a peer.
func NewPeerInfo(p peer.Peer) *PeerInfo {
	pi := &PeerInfo{}
	pi.PeerId = peer.IDB58Encode(p.GetPeerID())
	return pi
}
