package link_establish_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver/v4"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "bifrost/link/establish"

// Controller is the static Link establish controller.
// It adds a EstablishLink for each configured peer.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// peers is the list of peers
	peers []peer.ID

	// srcPeerID is the src peer id
	// can be empty
	srcPeerID peer.ID
}

// NewController constructs a new peer controller.
// If privKey is nil, one will be generated.
func NewController(b bus.Bus, le *logrus.Entry, peers []peer.ID, srcID peer.ID) *Controller {
	return &Controller{
		le:    le,
		bus:   b,
		peers: peers,

		srcPeerID: srcID,
	}
}

// Execute executes the controller goroutine.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	for _, peerID := range c.peers {
		le := c.le.WithField("peer-id", peerID.String())
		_, diRef, err := c.bus.AddDirective(
			link.NewEstablishLinkWithPeer(c.srcPeerID, peerID),
			nil,
		)
		if err != nil {
			le.WithError(err).Warn("cannot establish link with configured peer")
			continue
		}
		le.Info("establishing link with configured peer")
		defer diRef.Release()
	}
	<-ctx.Done()
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
	return nil, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"link establish controller",
	)
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
