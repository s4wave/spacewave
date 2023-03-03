package bldr_plugin_builder_controller

import (
	"context"

	plugin "github.com/aperturerobotics/bldr/plugin"
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
	if c.c.GetBuilderConfig().GetPluginManifestMeta().GetPluginId() != pluginID {
		return nil
	}
	return &fetchPluginResolver{c: c, di: di}
}

// fetchPluginResolver resolves FetchPlugin with the controller.
type fetchPluginResolver struct {
	// c is the controller
	c *Controller
	// di is the directive instance
	di directive.Instance
}

// Resolve resolves the values, emitting them to the handler.
func (r *fetchPluginResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	_ = handler.ClearValues()
	res, err := r.c.GetResultPromise().Await(ctx)
	if err != nil {
		return err
	}
	var value plugin_host.FetchPluginValue = &plugin.FetchPluginResponse{
		PluginManifest: res.PluginManifestRef,
	}
	_, _ = handler.AddValue(value)
	return nil
}

// _ is a type assertion
var _ directive.Resolver = ((*fetchPluginResolver)(nil))
