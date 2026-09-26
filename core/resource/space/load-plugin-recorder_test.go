package resource_space

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// loadPluginRecorder records LoadPlugin directives and resolves them idle.
type loadPluginRecorder struct {
	// loads receives each LoadPlugin directive.
	loads chan bldr_plugin.LoadPlugin
}

// newLoadPluginRecorder constructs a LoadPlugin recorder.
func newLoadPluginRecorder() *loadPluginRecorder {
	return &loadPluginRecorder{loads: make(chan bldr_plugin.LoadPlugin, 1)}
}

// GetControllerInfo returns information about the controller.
func (c *loadPluginRecorder) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		"test/load-plugin-recorder",
		controller.MustParseVersion("0.0.1"),
		"records plugin loads",
	)
}

// Execute executes the controller.
func (c *loadPluginRecorder) Execute(context.Context) error {
	return nil
}

// HandleDirective records LoadPlugin directives.
func (c *loadPluginRecorder) HandleDirective(
	_ context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	load, ok := inst.GetDirective().(bldr_plugin.LoadPlugin)
	if !ok {
		return nil, nil
	}
	c.loads <- load
	return directive.R(directive.NewFuncResolver(func(_ context.Context, handler directive.ResolverHandler) error {
		handler.MarkIdle(true)
		return nil
	}), nil)
}

// Close releases any resources used by the controller.
func (c *loadPluginRecorder) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*loadPluginRecorder)(nil)
