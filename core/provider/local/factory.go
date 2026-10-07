package provider_local

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_controller "github.com/s4wave/spacewave/core/provider/controller"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// Factory constructs the local provider.
type Factory struct {
	// bus is the controller bus
	bus bus.Bus
	// transportOptions configure each Session's independently owned transports.
	transportOptions []transport.SessionTransportOption
}

// NewFactory builds the factory.
func NewFactory(bus bus.Bus, transportOptions ...transport.SessionTransportOption) *Factory {
	return &Factory{bus: bus, transportOptions: slices.Clone(transportOptions)}
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
	// Take the construct logger and the local provider config.
	le := opts.GetLogger()
	cc := conf.(*Config)

	// Reject a peer id that does not parse.
	if cc.GetPeerId() != "" {
		if _, err := cc.ParsePeerID(); err != nil {
			return nil, err
		}
	}

	// Default an empty provider id to the local provider id.
	providerID := cc.GetProviderId()
	if providerID == "" {
		providerID = ProviderID
	}

	// The local provider does not use a provider-level Peer. Account and session
	// operations resolve the volume peer through their volume.
	return provider_controller.NewProviderControllerWithoutPeer(
		le,
		t.bus,
		controller.NewInfo(ControllerID, Version, controllerDescrip),
		NewProviderInfo(providerID),
		func(
			ctx context.Context,
			le *logrus.Entry,
			info *provider.ProviderInfo,
			peer peer.Peer,
			handler provider.ProviderHandler,
		) (provider.Provider, error) {
			return NewProvider(
				le,
				t.bus,
				cc.GetStorageId(),
				cc.GetSignalingUrl(),
				cc.GetSignalingEnvPrefix(),
				info,
				peer,
				handler,
				t.transportOptions...,
			), nil
		},
	), nil
}

// GetVersion returns the version of this controller.
func (t *Factory) GetVersion() controller.Version {
	return Version
}

// _ is a type assertion
var _ controller.Factory = (*Factory)(nil)
