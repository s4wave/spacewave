package floodsub_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/blang/semver/v4"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/pubsub"
	pubsub_controller "github.com/s4wave/spacewave/net/pubsub/controller"
	"github.com/s4wave/spacewave/net/pubsub/floodsub"
	"github.com/sirupsen/logrus"
)

// Factory constructs a floodsub controller.
type Factory struct {
	// bus is the controller bus
	bus bus.Bus
}

// NewFactory builds a FloodSub controller factory.
// Similar to libp2p floodsub: sends messages to neighbors.
func NewFactory(bus bus.Bus) *Factory {
	return &Factory{bus: bus}
}

// GetConfigID returns the configuration ID for the controller.
func (t *Factory) GetConfigID() string {
	return ConfigID
}

// GetControllerID returns the unique ID for the controller.
func (t *Factory) GetControllerID() string {
	return ControllerID
}

// ConstructConfig constructs an instance of the controller configuration.
func (t *Factory) ConstructConfig() config.Config {
	return &Config{}
}

// Construct constructs the associated controller given configuration.
func (t *Factory) Construct(
	ctx context.Context,
	conf config.Config,
	opts controller.ConstructOpts,
) (controller.Controller, error) {
	le := opts.GetLogger()
	cc := conf.(*Config)

	// Construct the EntityGraph controller.
	return pubsub_controller.NewController(
		le,
		t.bus,
		controller.NewInfo(ControllerID, Version, "floodsub controller"),
		peer.ID(""), // floodsub does not bind to a specific peer id (yet)
		floodsub.FloodSubID,
		func(
			ctx context.Context,
			le *logrus.Entry,
			peer peer.Peer,
			handler pubsub.PubSubHandler,
		) (pubsub.PubSub, error) {
			return floodsub.NewFloodSub(
				ctx,
				le,
				handler,
				cc.GetFloodsubConfig(),
			)
		},
	), nil
}

// GetVersion returns the version of this controller.
func (t *Factory) GetVersion() semver.Version {
	return Version
}

// _ is a type assertion
var _ controller.Factory = ((*Factory)(nil))
