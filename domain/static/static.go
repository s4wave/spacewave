package identity_domain_static

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/identity"

	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "aperturerobotics/identity/domain/static/1"

// Controller implements the static identity domain controller.
// Serves identity lookup requests with a static list.
type Controller struct {
	// le is the log entry
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the configuration
	conf *Config
}

// NewController constructs a new auth challenge server.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	return &Controller{
		le:   le,
		bus:  bus,
		conf: conf,
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"identity static entity list",
	)
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any exceptional errors are returned for logging.
// It is safe to add a reference to the directive during this call.
// The context passed is canceled when the directive instance expires.
func (c *Controller) HandleDirective(
	ctx context.Context,
	inst directive.Instance,
) (directive.Resolver, error) {
	dir := inst.GetDirective()
	switch d := dir.(type) {
	case identity.IdentityLookupEntity:
		return c.resolveLookupEntity(ctx, inst, d)
	}

	return nil, nil
}

// Execute executes the given controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	// noop
	return nil
}

// LookupEntity looks up an entity record.
// returns nil if not found
func (c *Controller) LookupEntity(domainID, entityID string) (*identity.Entity, error) {
	if domains := c.conf.GetDomains(); len(domains) != 0 {
		_, found := checkDomainsList(domains, domainID)
		if !found {
			return nil, nil
		}
	}
	var selEnt *identity.Entity
	for _, ent := range c.conf.GetEntities() {
		if ent.GetEntityId() == entityID && ent.GetDomainId() == domainID {
			if selEnt == nil || selEnt.GetEpoch() < ent.GetEpoch() {
				selEnt = ent
			}
		}
	}
	return selEnt, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
