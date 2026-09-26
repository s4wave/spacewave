package plugin_host_mock

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
)

// LookupErrorController fails every LookupPluginHost with an error.
type LookupErrorController struct {
	// err is the lookup error.
	err error
}

// NewLookupErrorController constructs a controller that fails LookupPluginHost
// with err.
func NewLookupErrorController(err error) *LookupErrorController {
	return &LookupErrorController{err: err}
}

// GetControllerInfo returns information about the controller.
func (c *LookupErrorController) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		"bldr/plugin/host/mock/lookup-error",
		controller.MustParseVersion("0.0.1"),
		"fails plugin host lookups",
	)
}

// Execute executes the controller.
func (c *LookupErrorController) Execute(context.Context) error {
	return nil
}

// HandleDirective fails LookupPluginHost.
func (c *LookupErrorController) HandleDirective(
	_ context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	if _, ok := inst.GetDirective().(plugin_host.LookupPluginHost); !ok {
		return nil, nil
	}
	return directive.R(directive.NewFuncResolver(func(context.Context, directive.ResolverHandler) error {
		return c.err
	}), nil)
}

// Close releases any resources used by the controller.
func (c *LookupErrorController) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*LookupErrorController)(nil)
