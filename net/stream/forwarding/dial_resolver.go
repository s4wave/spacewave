package stream_forwarding

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	ma "github.com/aperturerobotics/go-multiaddr"
	"github.com/s4wave/spacewave/net/link"
	"github.com/sirupsen/logrus"
)

// DialResolver resolves HandleMountedStream by dialing a multiaddr.
type DialResolver struct {
	// le is the logger
	le *logrus.Entry
	// ma is the multiaddress to dial
	ma ma.Multiaddr
	// bus is the controller bus
	bus bus.Bus
}

// NewDialResolver constructs a new dial resolver.
func NewDialResolver(le *logrus.Entry, bus bus.Bus, ma ma.Multiaddr) (*DialResolver, error) {
	return &DialResolver{le: le, ma: ma, bus: bus}, nil
}

// Resolve resolves the values, emitting them to the handler.
func (r *DialResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	h, err := NewMountedStreamHandler(r.le, r.bus, r.ma)
	if err != nil {
		return err
	}

	handler.AddValue(link.MountedStreamHandler(h))
	return nil
}

// _ is a type assertion
var _ directive.Resolver = ((*DialResolver)(nil))
