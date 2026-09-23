package transport_controller

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/stream"
	"github.com/s4wave/spacewave/net/transport"
	"github.com/sirupsen/logrus"
)

// mountedLink implements mounted link
type mountedLink struct {
	c    *Controller
	tpt  transport.Transport
	link link.Link
}

// newMountedLink constructs a new mounted link for a transport link.
func newMountedLink(
	c *Controller,
	tpt transport.Transport,
	link link.Link,
) *mountedLink {
	return &mountedLink{
		c:    c,
		tpt:  tpt,
		link: link,
	}
}

// GetLinkUUID returns the underlying physical connection's ID.
func (l *mountedLink) GetLinkUUID() uint64 {
	return l.link.GetUUID()
}

// GetTransportUUID returns the unique ID of the transport.
func (l *mountedLink) GetTransportUUID() uint64 {
	return l.link.GetTransportUUID()
}

// GetRemoteTransportUUID returns the reported remote transport UUID.
// This should be negotiated in the handshake.
func (l *mountedLink) GetRemoteTransportUUID() uint64 {
	return l.link.GetRemoteTransportUUID()
}

// GetLocalPeer returns the identity of the local peer.
func (l *mountedLink) GetLocalPeer() peer.ID {
	return l.link.GetLocalPeer()
}

// GetRemotePeer returns the identity of the remote peer.
func (l *mountedLink) GetRemotePeer() peer.ID {
	return l.link.GetRemotePeer()
}

// OpenMountedStream opens a stream on the link, with the given parameters.
func (l *mountedLink) OpenMountedStream(
	ctx context.Context,
	protocolID protocol.ID,
	opts stream.OpenOpts,
) (link.MountedStream, error) {
	// Build the establishment header for the requested protocol.
	estMsg := NewStreamEstablish(protocolID)
	estMsg.Unreliable = opts.Unreliable

	// Open the underlying stream on the link.
	strm, err := l.link.OpenStream(opts)
	if err != nil {
		return nil, err
	}

	// A message stream negotiates on its reliable control stream.
	est := strm
	if msgs, ok := strm.(stream.MessageStream); ok {
		est = msgs.Control()
	} else if opts.Unreliable {
		_ = strm.Close()
		return nil, errors.New("link does not support unreliable streams")
	}

	// Bound header negotiation by a write deadline.
	_ = est.SetWriteDeadline(time.Now().Add(streamEstablishTimeout))

	// Write the establishment header and close failed streams.
	if _, err := writeStreamEstablishHeader(est, estMsg); err != nil {
		_ = strm.Close()
		return nil, err
	}

	// Clear the negotiation deadline after the header is sent.
	_ = est.SetDeadline(time.Time{})

	// Log the mounted stream when verbose transport logging is enabled.
	if l.c.verbose {
		l.c.le.
			WithFields(logrus.Fields{
				"link-id":     l.link.GetUUID(),
				"protocol-id": protocolID,
				"src-peer":    l.link.GetLocalPeer().String(),
				"dst-peer":    l.link.GetRemotePeer().String(),
			}).
			Debug("opened stream with peer")
	}

	// Return the stream with its negotiated protocol metadata.
	return newMountedStream(strm, opts, protocolID, l), nil
}

// _ is a type assertion.
var _ link.MountedLink = (*mountedLink)(nil)
