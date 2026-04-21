package tptaddr_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver/v4"
	"github.com/s4wave/spacewave/net/link"
	"github.com/sirupsen/logrus"
)

// ControllerID is the ID of the controller.
const ControllerID = "bifrost/tptaddr/controller"

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// Controller implements the tptaddr dialer controller.
// Resolves EstablishLinkWithPeer directives by creating LookupTptAddr and DialTptAddr directives.
type Controller struct {
	// le is the logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the config
	conf *Config
}

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	return &Controller{
		le:   le,
		conf: conf,
		bus:  bus,
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"lookup and dial peer transport address",
	)
}

// Execute executes the forwarding controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any unexpected errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir := di.GetDirective()
	if d, ok := dir.(link.EstablishLinkWithPeer); ok {
		return c.resolveEstablishLinkWithPeer(ctx, di, d)
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
