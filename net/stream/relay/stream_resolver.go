package stream_relay

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/sirupsen/logrus"
)

// RelayResolver resolves HandleMountedStream by relaying to a target peer.
type RelayResolver struct {
	// le is the logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// targetPeerID is the peer ID to relay to
	targetPeerID peer.ID
	// targetProtocolID is the protocol ID to relay to
	targetProtocolID protocol.ID
}

// NewRelayResolver constructs a new relay resolver.
// targetProtocolID cannot be empty
func NewRelayResolver(le *logrus.Entry, bus bus.Bus, targetPeerID peer.ID, targetProtocolID protocol.ID) (*RelayResolver, error) {
	return &RelayResolver{le: le, bus: bus, targetPeerID: targetPeerID, targetProtocolID: targetProtocolID}, nil
}

// Resolve resolves the values, emitting them to the handler.
func (r *RelayResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	h, err := NewMountedStreamHandler(r.le, r.bus, r.targetPeerID, r.targetProtocolID)
	if err != nil {
		return err
	}

	handler.AddValue(link.MountedStreamHandler(h))
	return nil
}

// _ is a type assertion
var _ directive.Resolver = ((*RelayResolver)(nil))
