package identity_domain_static

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/blang/semver/v4"
	identity_domain "github.com/s4wave/spacewave/identity/domain"
	identity_domain_controller "github.com/s4wave/spacewave/identity/domain/controller"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "identity/domain/static"

// Controller is the controller type.
type Controller = identity_domain_controller.Controller

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	return identity_domain_controller.NewController(
		le,
		bus,
		ControllerID,
		Version,
		conf.GetDomainInfo(),
		conf.GetResolveSelectIdentityDomain(),
		func(ctx context.Context, le *logrus.Entry, handler identity_domain.Handler) (identity_domain.Domain, error) {
			if err := conf.Validate(); err != nil {
				return nil, err
			}
			return NewDomain(conf), nil
		},
	)
}
