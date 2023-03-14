package bldr_project_controller

import (
	"context"

	plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/directive"
)

// resolveLoadPlugin resolves a LoadPlugin directive.
func (c *Controller) resolveLoadPlugin(
	ctx context.Context,
	di directive.Instance,
	dir plugin.LoadPlugin,
) directive.Resolver {
	pluginID := dir.LoadPluginID()
	manifestSet := c.c.GetProjectConfig().GetManifests()
	if _, ok := manifestSet[pluginID]; !ok {
		return nil
	}
	return &loadPluginResolver{c: c, di: di, pluginID: pluginID}
}

// loadPluginResolver resolves LoadPlugin with the controller.
type loadPluginResolver struct {
	// c is the controller
	c *Controller
	// di is the directive instance
	di directive.Instance
	// pluginID is the plugin identifier
	pluginID string
}

// Resolve resolves the values, emitting them to the handler.
func (r *loadPluginResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	// Load the plugin builder.
	pluginID := r.pluginID
	ref, _, _ := r.c.manifestBuilders.AddKeyRef(pluginID)

	// Release the reference when the directive is disposed.
	r.di.AddDisposeCallback(ref.Release)
	return nil
}

// _ is a type assertion
var _ directive.Resolver = ((*loadPluginResolver)(nil))
