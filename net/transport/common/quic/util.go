package transport_quic

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
)

// CheckAlreadyConnected checks if a address and peer id is already connected.
func CheckAlreadyConnected(t *Transport, addr string, peerID peer.ID) (bool, error) {
	lnk, ok := t.LookupLinkWithAddr(addr)
	if !ok {
		return false, nil
	}
	lnkPeer := lnk.GetRemotePeer().String()
	desiredPeer := peerID.String()
	if lnkPeer != desiredPeer {
		return false, errors.Errorf(
			"already connected to %s with different peer id: %s != requested %s",
			addr,
			lnkPeer,
			desiredPeer,
		)
	}
	return true, nil
}
