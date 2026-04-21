package pubsub_relay

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver/v4"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/pubsub"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "bifrost/pubsub/relay"

// Controller is the static Link establish controller.
// It adds a EstablishLink for each configured peer.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// peerID is the peer id
	peerID peer.ID
	// topics is the list of topics
	topics []string
}

// NewController constructs a new relay controllr.
func NewController(b bus.Bus, le *logrus.Entry, peerID peer.ID, topics []string) *Controller {
	return &Controller{
		le:     le,
		bus:    b,
		peerID: peerID,
		topics: topics,
	}
}

// Execute executes the controller goroutine.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	cpeer, _, ref, err := peer.GetPeerWithID(ctx, c.bus, c.peerID, false, nil)
	if err != nil {
		return err
	}
	if ref != nil {
		defer ref.Release()
	}

	privKey, err := cpeer.GetPrivKey(ctx)
	if err != nil {
		return err
	}

	for _, topic := range c.topics {
		le := c.le.WithField("topic", topic)
		_, diRef, err := c.bus.AddDirective(
			pubsub.NewBuildChannelSubscription(topic, privKey),
			nil,
		)
		if err != nil {
			le.WithError(err).Warn("cannot build pubsub directive")
			continue
		}
		le.Info("establishing pubsub subscription")
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
