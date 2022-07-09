package identity_domain_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/identity"
	aidentity "github.com/aperturerobotics/identity"
	identity_domain "github.com/aperturerobotics/identity/domain"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// Constructor constructs a Domain with common parameters.
type Constructor func(
	ctx context.Context,
	le *logrus.Entry,
	handler identity_domain.Handler,
) (identity_domain.Domain, error)

// Controller implements a common Domain controller.
type Controller struct {
	// ctx is the controller context
	// set in the execute() function
	// ensure not used before execute sets it.
	ctx context.Context
	// le is the logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// ctor is the constructor
	ctor Constructor

	// controllerID is the controller id
	controllerID string
	// ver is the controller version
	controllerVer semver.Version
	// domainID is the domain id
	domainID string

	// domainCh holds the domain like a bucket
	domainCh chan identity_domain.Domain
}

// NewController constructs a new identity domain controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	controllerID string,
	controllerVer semver.Version,
	domainID string,
	ctor Constructor,
) *Controller {
	return &Controller{
		le:       le,
		bus:      bus,
		ctor:     ctor,
		domainID: domainID,

		controllerID:  controllerID,
		controllerVer: controllerVer,

		domainCh: make(chan identity_domain.Domain, 1),
	}
}

// GetControllerID returns the controller ID.
func (c *Controller) GetControllerID() string {
	return c.controllerID
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		c.GetControllerID(),
		c.controllerVer,
		"identity domain controller "+c.domainID,
	)
}

// Execute executes the domain.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	c.ctx = ctx
	// Acquire a handle to the node.
	le := c.le.WithField("domain-id", c.domainID)

	// Construct the domain
	dm, err := c.ctor(
		ctx,
		le,
		c,
	)
	if err != nil {
		return err
	}
	defer dm.Close()
	c.domainCh <- dm

	c.le.Debug("executing identity domain controller")
	err = dm.Execute(ctx)
	if err == nil {
		// indicated success, wait for ctx cancel
		<-ctx.Done()
	}
	select {
	case <-c.domainCh:
	default:
	}
	return err
}

// GetDomain returns the controlled domain.
// This may be nil until the transport is constructed.
func (c *Controller) GetDomain(ctx context.Context) (identity_domain.Domain, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case dm := <-c.domainCh:
		c.domainCh <- dm
		return dm, nil
	}
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any exceptional errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) (directive.Resolver, error) {
	dir := di.GetDirective()
	switch d := dir.(type) {
	case identity_domain.LookupIdentityDomain:
		return c.resolveLookupIdentityDomain(ctx, di, d)
	case aidentity.SelectIdentityEntity:
		return c.resolveSelectEntity(ctx, di, d)
	case identity.IdentityLookupEntity:
		return c.resolveLookupEntity(ctx, di, d)
	}

	return nil, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
