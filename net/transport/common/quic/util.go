package transport_quic

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
)

// CheckAlreadyConnected checks if a address and peer id is already connected.
func CheckAlreadyConnected(t *Transport, addr string, peerID peer.ID) (bool, error) {
	// Look up an existing link for the requested address.
	lnk, ok := t.LookupLinkWithAddr(addr)
	if !ok {
		return false, nil
	}

	// Compare the existing link's peer identity with the requested peer.
	lnkPeer := lnk.GetRemotePeer().String()
	desiredPeer := peerID.String()

	// Reject an address already bound to a different peer.
	if lnkPeer != desiredPeer {
		return false, errors.Errorf(
			"already connected to %s with different peer id: %s != requested %s",
			addr,
			lnkPeer,
			desiredPeer,
		)
	}

	// Confirm that the existing link matches the requested peer.
	return true, nil
}
