package link

import (
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/stream"
)

// Link represents a one-hop connection between two peers.
type Link interface {
	// GetUUID returns the host-unique ID.
	// This should be repeatable between re-constructions of the same link.
	GetUUID() uint64
	// GetTransportUUID returns the unique ID of the transport.
	GetTransportUUID() uint64
	// OpenStream opens a stream on the link, with the given parameters.
	OpenStream(opts stream.OpenOpts) (stream.Stream, error)
	// AcceptStream accepts a stream from the link.
	// Terminates when the link closes.
	AcceptStream() (stream.Stream, stream.OpenOpts, error)
	// GetRemotePeer returns the identity of the remote peer.
	GetRemotePeer() peer.ID
	// GetLocalPeer returns the identity of the local peer.
	GetLocalPeer() peer.ID
	// GetRemoteTransportUUID returns the reported remote transport UUID.
	// This should be negotiated in the handshake.
	GetRemoteTransportUUID() uint64
	// Close closes the link.
	// Any blocked ReadFrom or WriteTo operations will be unblocked and return errors.
	// The link should call the HandleLinkLost callback exactly once.
	// Close may be called many times.
	Close() error
}
