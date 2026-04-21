package stream_forwarding

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	ma "github.com/aperturerobotics/go-multiaddr"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// Controller implements the forwarding controller. The controller handles
// HandleMountedStream directives by dialing a target multiaddress.
type Controller struct {
	// le is the logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the config
	conf *Config
	// dialMa is the dial multiaddr
	dialMa ma.Multiaddr
	// localPeerID is the peer ID to dial for
	localPeerID peer.ID
}

// NewController constructs a new forwarding controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) (*Controller, error) {
	dialMa, err := conf.ParseTargetMultiaddr()
	if err != nil {
		return nil, err
	}

	pid, err := conf.ParsePeerID()
	if err != nil {
		return nil, err
	}

	return &Controller{
		le:          le,
		conf:        conf,
		bus:         bus,
		dialMa:      dialMa,
		localPeerID: pid,
	}, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"forwarding controller",
	)
}

// Execute executes the forwarding controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	// For forwarding, we just handle directives directly.
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any unexpected errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir := di.GetDirective()
	// HandleMountedStream handler.
	if d, ok := dir.(link.HandleMountedStream); ok {
		return c.resolveHandleMountedStream(ctx, di, d)
	}

	return nil, nil
}

// resolveHandleMountedStream resolves a HandleMountedStream directive by dialing a target.
func (c *Controller) resolveHandleMountedStream(
	ctx context.Context,
	di directive.Instance,
	dir link.HandleMountedStream,
) ([]directive.Resolver, error) {
	if c.conf.GetProtocolId() != "" &&
		c.conf.GetProtocolId() != string(dir.HandleMountedStreamProtocolID()) {
		return nil, nil
	}
	if localPeerID := c.localPeerID; localPeerID != peer.ID("") {
		if lid := dir.HandleMountedStreamLocalPeerID(); lid != localPeerID {
			c.le.Debugf(
				"incoming stream %s != filtered %s",
				lid.String(),
				localPeerID.String(),
			)
			return nil, nil
		}
	}
	dialRes, err := NewDialResolver(c.le, c.bus, c.dialMa)
	if err != nil {
		return nil, err
	}
	return directive.Resolvers(dialRes), nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
