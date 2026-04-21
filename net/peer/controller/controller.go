package peer_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver/v4"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "bifrost/peer"

// Controller is the Peer controller.
// It implements peer.Peer as a controller.
// It implements a "localhost" loopback transport for the peer.
type Controller struct {
	// Peer is the underlying peer
	peer.Peer
	// le is the root logger
	le *logrus.Entry
}

// NewController constructs a new peer controller.
// If privKey is nil, one will be generated.
func NewController(le *logrus.Entry, p peer.Peer) *Controller {
	return &Controller{
		Peer: p,
		le:   le,
	}
}

// Execute executes the controller goroutine.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	c.le.WithField("peer-id", c.GetPeerID().String()).Debug("peer mounted")
	// TODO: loopback controller
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any unexpected errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	dir := di.GetDirective()
	switch d := dir.(type) {
	case peer.GetPeer:
		return c.resolveGetPeer(d), nil
	}

	return nil, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"peer controller "+c.GetPeerID().String(),
	)
}

// resolveGetPeer resolves the GetPeer directive
func (c *Controller) resolveGetPeer(d peer.GetPeer) []directive.Resolver {
	res := peer.NewGetPeerResolver(d, c)
	if res == nil {
		return nil
	}
	return directive.Resolvers(res)
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var (
	_ controller.Controller = ((*Controller)(nil))
	_ peer.Peer             = ((*Controller)(nil))
)
