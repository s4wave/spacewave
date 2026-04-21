package udp

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/blang/semver/v4"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/transport"
	tc "github.com/s4wave/spacewave/net/transport/controller"
	"github.com/sirupsen/logrus"
)

// Factory constructs a UDP transport.
type Factory struct {
	// bus is the controller bus
	bus bus.Bus
}

// NewFactory builds a UDP transport factory.
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

	peerIDConstraint, err := cc.ParseTransportPeerID()
	if err != nil {
		return nil, err
	}

	// Construct the transport controller.
	return tc.NewController(
		le,
		t.bus,
		controller.NewInfo(ControllerID, Version, "udp transport"),
		peerIDConstraint,
		cc.GetVerbose(),
		func(
			ctx context.Context,
			le *logrus.Entry,
			pkey crypto.PrivKey,
			handler transport.TransportHandler,
		) (transport.Transport, error) {
			return NewUDP(
				ctx,
				le,
				pkey,
				handler,
				cc.GetPacketOpts(),
				0,
				cc.GetListenAddr(),
				cc.GetDialers(),
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
