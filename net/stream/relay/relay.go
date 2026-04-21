package stream_relay

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

// Controller implements the relay controller. The controller handles
// HandleMountedStream directives by relaying the stream to a target peer ID.
type Controller struct {
	// le is the logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the config
	conf *Config
	// srcPeerID is the source peer ID to filter on
	srcPeerID peer.ID
	// protocolID is the protocol id to listen on
	protocolID protocol.ID
	// targetPeerID is the target peer ID to relay to
	targetPeerID peer.ID
	// targetProtocolID is the target protocol ID to relay to
	targetProtocolID protocol.ID
}

// NewController constructs a new relay controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) (*Controller, error) {
	spid, err := conf.ParsePeerID()
	if err != nil {
		return nil, err
	}
	if len(spid) == 0 {
		return nil, peer.ErrEmptyPeerID
	}

	tpid, err := conf.ParseTargetPeerID()
	if err != nil {
		return nil, err
	}
	if len(tpid) == 0 {
		return nil, peer.ErrEmptyPeerID
	}

	srcProtocolID := protocol.ID(conf.GetProtocolId())
	if err := srcProtocolID.Validate(); err != nil {
		return nil, err
	}

	targetProtocolID := protocol.ID(conf.GetTargetProtocolId())
	if targetProtocolID == "" {
		targetProtocolID = srcProtocolID
	} else if err := targetProtocolID.Validate(); err != nil {
		return nil, err
	}

	return &Controller{
		le:               le,
		conf:             conf,
		bus:              bus,
		srcPeerID:        spid,
		protocolID:       srcProtocolID,
		targetPeerID:     tpid,
		targetProtocolID: targetProtocolID,
	}, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"relay controller",
	)
}

// Execute executes the relay controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	// For relay, we just handle directives directly.
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
		return c.resolveHandleMountedStream(d)
	}

	return nil, nil
}

// resolveHandleMountedStream resolves a HandleMountedStream directive by dialing a target.
func (c *Controller) resolveHandleMountedStream(dir link.HandleMountedStream) ([]directive.Resolver, error) {
	if c.conf.GetProtocolId() != string(dir.HandleMountedStreamProtocolID()) ||
		c.srcPeerID != dir.HandleMountedStreamLocalPeerID() {
		return nil, nil
	}

	return directive.R(NewRelayResolver(c.le, c.bus, c.targetPeerID, c.targetProtocolID))
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
