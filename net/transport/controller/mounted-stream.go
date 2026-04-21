package transport_controller

import (
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/stream"
)

// mountedStream implements mounted stream from link
type mountedStream struct {
	strm         stream.Stream
	strmOpenOpts stream.OpenOpts
	protocolID   protocol.ID
	linkPeer     peer.ID
	link         link.MountedLink
}

func newMountedStream(
	strm stream.Stream,
	strmOpenOpts stream.OpenOpts,
	protocolID protocol.ID,
	link link.MountedLink,
) *mountedStream {
	return &mountedStream{
		strm:         strm,
		strmOpenOpts: strmOpenOpts,
		protocolID:   protocolID,
		link:         link,
		linkPeer:     link.GetRemotePeer(),
	}
}

// GetStream returns the underlying stream object.
func (m *mountedStream) GetStream() stream.Stream {
	return m.strm
}

// GetProtocolID returns the protocol ID of the stream.
func (m *mountedStream) GetProtocolID() protocol.ID {
	return m.protocolID
}

// GetOpenOpts returns the options used to open the stream.
func (m *mountedStream) GetOpenOpts() stream.OpenOpts {
	return m.strmOpenOpts
}

// GetPeerID returns the peer ID for the other end of the stream.
func (m *mountedStream) GetPeerID() peer.ID {
	return m.linkPeer
}

// GetLink returns the associated link carrying the stream.
func (m *mountedStream) GetLink() link.MountedLink {
	return m.link
}

// _ is a type assertion.
var _ link.MountedStream = ((*mountedStream)(nil))
