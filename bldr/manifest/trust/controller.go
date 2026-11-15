package bldr_manifest_trust

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
)

// ControllerID is the controller identifier.
const ControllerID = "bldr/manifest/trust"

// Version is the controller version.
var Version = controller.MustParseVersion("0.0.1")

// Controller resolves LookupTrustedRoots with one Set. The host constructs it
// in code, never from a config, so a config set cannot start a second one.
type Controller struct {
	// set is the trusted root set.
	set *Set
}

// NewController constructs a Controller serving set.
func NewController(set *Set) *Controller {
	return &Controller{set: set}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "trusted manifest roots")
}

// Execute executes the controller.
func (c *Controller) Execute(context.Context) error {
	return nil
}

// HandleDirective resolves LookupTrustedRoots with the set.
func (c *Controller) HandleDirective(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	switch inst.GetDirective().(type) {
	case LookupTrustedRoots:
		return directive.R(directive.NewValueResolver([]LookupTrustedRootsValue{c.set}), nil)
	}
	return nil, nil
}

// Close releases any resources used by the controller.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)
