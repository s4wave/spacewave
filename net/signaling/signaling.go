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
}
