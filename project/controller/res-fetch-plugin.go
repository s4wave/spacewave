package bldr_project_controller

import (
	"context"

	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	"github.com/aperturerobotics/controllerbus/directive"
)

// resolveFetchPlugin resolves a FetchPlugin directive.
func (c *Controller) resolveFetchPlugin(
	ctx context.Context,
	di directive.Instance,
	dir plugin_host.FetchPlugin,
) directive.Resolver {
	pluginID := dir.FetchPluginID()
	pluginSet := c.c.GetProjectConfig().GetPlugin()
	if _, ok := pluginSet[pluginID]; !ok {
		return nil
	}
	return &fetchPluginResolver{c: c, di: di, pluginID: pluginID}
}

// fetchPluginResolver resolves FetchPlugin with the controller.
type fetchPluginResolver struct {
	// c is the controller
	c *Controller
	// di is the directive instance
	di directive.Instance
	// pluginID is the plugin identifier
	pluginID string
}

// Resolve resolves the values, emitting them to the handler.
func (r *fetchPluginResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	// Load the plugin builder.
	pluginID := r.pluginID
	ref, _, _ := r.c.pluginBuilders.AddKeyRef(pluginID)

	// Release the reference when the directive is disposed.
	r.di.AddDisposeCallback(ref.Release)
	return nil
}

// _ is a type assertion
var _ directive.Resolver = ((*fetchPluginResolver)(nil))
