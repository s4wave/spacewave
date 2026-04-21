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
	srcPeer, err := c.ParseSrcPeerId()
	if err != nil {
		return nil, errors.Wrap(err, "src_peer_id")
	}

	serverPeerIDs, err := c.ParseServerPeerIds()
	if err != nil {
		return nil, errors.Wrap(err, "src_peer_id")
	}

	timeoutDur, err := c.ParseTimeoutDur()
	if err != nil {
		return nil, errors.Wrap(err, "timeout_dur")
	}

	if err := protocolID.Validate(); err != nil {
		return nil, err
	}

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
