package plugin_host_controller

import (
	"context"

	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	"github.com/aperturerobotics/controllerbus/directive"
)

// resolveLoadPlugin resolves a LoadPlugin directive.
func (c *Controller) resolveLoadPlugin(
	ctx context.Context,
	di directive.Instance,
	dir plugin_host.LoadPlugin,
) (directive.Resolver, error) {
	pluginID := dir.LoadPluginID()
	return &loadPluginResolver{c: c, pluginID: pluginID}, nil
}

// loadPluginResolver resolves LoadPlugin with the controller.
type loadPluginResolver struct {
	// c is the controller
	c *Controller
	// pluginID is the plugin identifier
	pluginID string
}

// Resolve resolves the values, emitting them to the handler.
func (r *loadPluginResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	var id uint32
	var added bool
	return r.c.LoadPlugin(ctx, r.pluginID, func(ps *plugin_host.PluginStateSnapshot) error {
		if added {
			handler.RemoveValue(id)
		}
		var val plugin_host.LoadPluginValue = ps
		id, added = handler.AddValue(val)
		return nil
	})
}

// _ is a type assertion
var _ directive.Resolver = ((*loadPluginResolver)(nil))
