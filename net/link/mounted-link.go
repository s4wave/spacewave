package link

import (
	"context"

	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/stream"
)

// MountedLink is a Link managed by the transport controller.
type MountedLink interface {
	// GetLinkUUID returns the host-unique link ID.
	// This should be repeatable between re-constructions of the same link.
	GetLinkUUID() uint64

	// GetTransportUUID returns the unique ID of the transport.
	GetTransportUUID() uint64
	// GetRemoteTransportUUID returns the reported remote transport UUID.
	// This should be negotiated in the handshake.
	GetRemoteTransportUUID() uint64

	// GetLocalPeer returns the identity of the local peer.
	GetLocalPeer() peer.ID
	// GetRemotePeer returns the identity of the remote peer.
	GetRemotePeer() peer.ID

	// OpenMountedStream opens a stream on the link, with the given parameters.
	OpenMountedStream(
		ctx context.Context,
		protocolID protocol.ID,
		opts stream.OpenOpts,
	) (MountedStream, error)
}
