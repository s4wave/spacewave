// Package signaling carries small messages between two peers that have no link
// yet, so they can set one up, as WebRTC does with its offer and answer.
//
// The SignalPeer directive opens a SignalPeerSession with a remote peer over a
// signaling channel, optionally a named one. Send blocks until the remote peer
// acknowledges the message or the context ends, and Recv acknowledges each
// message it returns. GetPresence reports a presence epoch that is zero while
// the remote peer is absent and takes a new value each time it joins, so a
// caller can tell a rejoin from a peer that stayed. HandleSignalPeer is the
// inbound side: it hands a session that a remote peer opened to the
// controller that serves its signaling ID.
package signaling

import (
	"context"

	"github.com/s4wave/spacewave/net/peer"
)

// SignalPeerSession is a handle to a Signaling session with a remote peer.
type SignalPeerSession interface {
	// GetLocalPeerID returns the local peer ID.
	GetLocalPeerID() peer.ID
	// GetRemotePeerID returns the remote peer ID.
	GetRemotePeerID() peer.ID

	// Send transmits a message to the remote peer.
	// Blocks until the context is canceled OR the message is acked.
	Send(ctx context.Context, msg []byte) error
	// Recv waits for and acknowledges an incoming message from the remote peer.
	Recv(ctx context.Context) ([]byte, error)

	// GetPresence returns the presence epoch of the remote peer and a
	// channel closed when the epoch may have changed. The epoch is zero
	// while the remote peer is absent from the session, and each time it
	// joins the epoch takes a value this session has not reported before.
	GetPresence() (epoch uint64, changed <-chan struct{})
}
