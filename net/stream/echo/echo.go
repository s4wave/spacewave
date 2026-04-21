package stream_echo

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/sirupsen/logrus"
)

// DefaultProtocolID is the default echo protocol ID.
var DefaultProtocolID = protocol.ID("bifrost/echo")

// Controller implements the stream echo controller. The controller handles
// HandleMountedStream directives by echoing all data.
type Controller struct {
	// le is the logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the config
	conf *Config
	// localPeerID is the peer ID to echo for
	localPeerID peer.ID
}

// NewController constructs a new echoing controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) (*Controller, error) {
	pid, err := conf.ParsePeerID()
	if err != nil {
		return nil, err
	}

	if conf.GetProtocolId() == "" {
		conf.ProtocolId = string(DefaultProtocolID)
	}

	return &Controller{
		le:          le,
		conf:        conf,
		bus:         bus,
		localPeerID: pid,
	}, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"echo controller",
	)
}

// Execute executes the echo controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any unexpected errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir := di.GetDirective()
	if d, ok := dir.(link.HandleMountedStream); ok {
		return c.resolveHandleMountedStream(ctx, di, d)
	}

	return nil, nil
}

// resolveHandleMountedStream resolves a HandleMountedStream directive by echoing data.
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
	c.le.
		WithField("local-peer", dir.HandleMountedStreamLocalPeerID().String()).
		WithField("remote-peer", dir.HandleMountedStreamRemotePeerID().String()).
		WithField("protocol-id", dir.HandleMountedStreamProtocolID()).
		Debug("starting echo stream handler")
	res, err := NewEchoResolver(c.le, c.bus)
	if err != nil {
		return nil, err
	}
	return directive.Resolvers(res), nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
