package identity_domain_aperture

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/identity"
	identity_domain "github.com/aperturerobotics/identity/domain"
	identity_domain_controller "github.com/aperturerobotics/identity/domain/controller"
	identity_domain_client "github.com/aperturerobotics/identity/domain/service/client"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// Version is the api version
var Version = semver.MustParse("0.0.1")

// Factory constructs a aperture auth domain controller.
type Factory struct {
	// bus is the controller bus
	bus bus.Bus
}

// NewFactory builds the aperture identity domain factory.
func NewFactory(bus bus.Bus) *Factory {
	return &Factory{bus: bus}
}

// GetConfigID returns the configuration ID for the controller.
func (t *Factory) GetConfigID() string {
	return ConfigID
}

// ConstructConfig constructs an instance of the controller configuration.
func (t *Factory) ConstructConfig() config.Config {
	return &Config{}
}

// Construct constructs the associated controller given configuration.
// The transport's identity (private key) comes from a GetNode lookup.
func (t *Factory) Construct(
	conf config.Config,
	opts controller.ConstructOpts,
) (controller.Controller, error) {
	le := opts.GetLogger()
	cc := conf.(*Config)

	// Construct the domain controller.
	domainID := cc.GetDomainInfo().GetDomainId()
	if err := identity.ValidateDomainID(domainID); err != nil {
		return nil, err
	}
	if cc.IdentityClient == nil {
		cc.IdentityClient = &identity_domain_client.Config{}
	}
	cc.IdentityClient.DomainIds = []string{domainID}

	return identity_domain_controller.NewController(le, t.bus, domainID, Version, func(
		ctx context.Context,
		le *logrus.Entry,
		handler identity_domain.Handler,
	) (identity_domain.Domain, error) {
		return NewApertureAuth(le, t.bus, cc)
	}), nil
}

// GetVersion returns the version of this controller.
func (t *Factory) GetVersion() semver.Version {
	return Version
}

// _ is a type assertion
var _ controller.Factory = ((*Factory)(nil))
