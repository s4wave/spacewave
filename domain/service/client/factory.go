package identity_domain_client

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/identity"
	identity_domain "github.com/aperturerobotics/identity/domain"
	identity_domain_controller "github.com/aperturerobotics/identity/domain/controller"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// Factory constructs a domain client controller.
type Factory struct {
	// bus is the controller bus
	bus bus.Bus
}

// NewFactory builds a controller factory.
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

	domainInfo := cc.GetDomainInfo()
	domainID := domainInfo.GetDomainId()
	if err := identity.ValidateDomainID(domainID); err != nil {
		return nil, err
	}

	// Construct the controller.
	return identity_domain_controller.NewController(
		le,
		t.bus,
		ControllerID,
		Version,
		domainInfo,
		cc.GetResolveSelectIdentityDomain(),
		func(
			ctx context.Context,
			le *logrus.Entry,
			handler identity_domain.Handler,
		) (identity_domain.Domain, error) {
			return NewDomain(le, t.bus, cc)
		},
	), nil
}

// GetVersion returns the version of this controller.
func (t *Factory) GetVersion() semver.Version {
	return Version
}

// _ is a type assertion
var _ controller.Factory = ((*Factory)(nil))
