package auth_derive

import (
	"context"

	auth_method_triplesec "github.com/aperturerobotics/auth/method/triplesec"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/identity"
	"github.com/blang/semver/v4"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "auth/derive"

// Controller is the derive key controller.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// c is the config
	c *Config
}

// NewController constructs a new terminal ui
func NewController(b bus.Bus, le *logrus.Entry, c *Config) (*Controller, error) {
	return &Controller{
		le:  le,
		bus: b,
		c:   c,
	}, nil
}

// AuthMethodIdSupported checks if the auth method id is known.
func AuthMethodIdSupported(id string) bool {
	return id == auth_method_triplesec.MethodID
}

// Execute executes the controller goroutine.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
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
	dir := di.GetDirective()
	switch d := dir.(type) {
	case identity.DeriveEntityKeypair:
		return directive.R(c.resolveDeriveEntityKeypair(ctx, di, d))
	}
	return nil, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"derive keypair controller",
	)
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
