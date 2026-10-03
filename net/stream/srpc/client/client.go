package stream_srpc_client

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/protocol"
	stream_srpc "github.com/s4wave/spacewave/net/stream/srpc"
	"github.com/sirupsen/logrus"
)

// Client is a common srpc client implementation.
type Client = srpc.Client

// NewClient constructs a new client.
func NewClient(le *logrus.Entry, b bus.Bus, c *Config, protocolID protocol.ID) (Client, error) {
	// Resolve the peer identity used to originate client streams.
	srcPeer, err := c.ParseSrcPeerId()
	if err != nil {
		return nil, errors.Wrap(err, "src_peer_id")
	}

	// Resolve the server peers available to the RPC client.
	serverPeerIDs, err := c.ParseServerPeerIds()
	if err != nil {
		return nil, errors.Wrap(err, "server_peer_ids")
	}

	// Parse the configured deadline for opening an RPC stream.
	timeoutDur, err := c.ParseTimeoutDur()
	if err != nil {
		return nil, errors.Wrap(err, "timeout_dur")
	}

	// Validate the protocol served by the remote RPC peers.
	if err := protocolID.Validate(); err != nil {
		return nil, err
	}

	// Build the stream opener with the configured peers and transport.
	openStreamFn := stream_srpc.NewMultiOpenStreamFunc(
		b,
		le,
		protocolID,
		srcPeer, serverPeerIDs,
		c.GetTransportId(),
		timeoutDur,
	)

	return srpc.NewClient(openStreamFn), nil
}
