// Package link is the authenticated connection between two peers and the
// directives that open and serve streams over it.
//
// A Link is one encrypted connection from a transport. Its UUID distinguishes
// separate connections to the same peer, and its remote transport UUID is
// learned in the handshake. Close is idempotent, unblocks pending I/O and
// reports the loss to the transport handler exactly once. A MountedLink is a
// Link after the transport controller has admitted it. OpenMountedStream opens
// a stream for one protocol.ID on it, and the controller writes the stream's
// protocol header before the caller writes data.
//
// EstablishLinkWithPeer asks the bus for a MountedLink to a target peer, from
// any local peer when the source is empty. The link stays open while the
// directive is referenced and for a hold-open interval after the last
// reference is released, so a burst of requests reuses one link.
// OpenStreamWithPeerEx establishes the link and opens a stream in one call.
// HandleMountedStream is the inbound side: the controller emits it for each
// accepted stream with the stream's protocol and peers, and the resolving
// MountedStreamHandler owns the stream from then on.
package link

import (
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/stream"
)

// Link represents a one-hop connection between two peers.
type Link interface {
	// GetUUID identifies this physical connection. Separate connections must
	// have distinct IDs. Wrappers of the same connection retain its ID.
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

// MessageLink is a Link that carries unreliable streams: OpenStream with
// OpenOpts.Unreliable returns a stream.MessageStream.
type MessageLink interface {
	Link

	// AcceptMessageStream binds the message plane of an accepted stream whose
	// opener set OpenOpts.Unreliable. control is the accepted stream, after
	// the stream's establishment was read from it.
	AcceptMessageStream(control stream.Stream) (stream.MessageStream, error)
}
