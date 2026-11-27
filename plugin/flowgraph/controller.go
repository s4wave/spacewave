package flowgraph_plugin

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	bldr_web_bundler "github.com/s4wave/spacewave/bldr/web/bundler"
	s4wave_plugin "github.com/s4wave/spacewave/sdk/plugin"
)

// ControllerID selects the Stream Echo plugin controller.
const ControllerID = "plugin/flowgraph"

// PluginID identifies the manifest shipping this node type and editor.
const PluginID = "spacewave-flowgraph"

// Version is the plugin controller version.
var Version = controller.MustParseVersion("0.0.1")

// EditorEntrypoint contains the editor asset URL supplied by the plugin builder.
//
//bldr:esbuild StreamEchoEditor.tsx
var EditorEntrypoint bldr_web_bundler.WebBundlerOutput

// Controller publishes the plugin's node type and editor until the bus removes it.
type Controller struct {
	// BusController supplies the plugin-local bus and lifetime.
	*bus.BusController[*Config]
}

// NewFactory constructs the plugin startup factory for Bldr's goPkgs discovery.
func NewFactory(b bus.Bus) controller.Factory {
	return bus.NewBusControllerFactory(
		b,
		ConfigID,
		ControllerID,
		Version,
		"Stream Echo node type and editor",
		func() *Config { return &Config{} },
		func(base *bus.BusController[*Config]) (*Controller, error) {
			return &Controller{BusController: base}, nil
		},
	)
}

// Execute registers the plugin capabilities over the core Resource boundary.
func (c *Controller) Execute(ctx context.Context) error {
	// Connect to the core Resources on the host without forwarding directives.
	resources, err := s4wave_plugin.ConnectPluginResources(ctx, c.GetBus(), "spacewave-core")
	if err != nil {
		return err
	}
	defer resources.Release()
	client, root, err := resources.OpenRootClient()
	if err != nil {
		return err
	}
	defer root.Release()

	// Hold the registrations while this plugin instance runs.
	release, err := Register(ctx, c.GetBus(), resources, client, EditorEntrypoint.GetEntrypointHref())
	if err != nil {
		return err
	}
	defer func() {
		if err := release(); err != nil {
			c.GetLogger().WithError(err).Warn("detach Stream Echo compiler")
		}
	}()
	<-ctx.Done()
	return nil
}

// _ is a type assertion.
var _ controller.Controller = (*Controller)(nil)
