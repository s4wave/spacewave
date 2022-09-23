package plugin_fetch

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
) (directive.Resolver, error) {
	pluginID := dir.FetchPluginID()
	if c.fetchPluginIdRe != nil {
		if !c.fetchPluginIdRe.MatchString(pluginID) {
			return nil, nil
		}
	}
	return &fetchPluginResolver{c: c, pluginID: pluginID}, nil
}

// fetchPluginResolver resolves FetchPlugin with the controller.
type fetchPluginResolver struct {
	// c is the controller
	c *Controller
	// pluginID is the plugin identifier
	pluginID string
}

// Resolve resolves the values, emitting them to the handler.
func (r *fetchPluginResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	res, err := r.c.FetchPlugin(ctx, r.pluginID)
	if err == nil {
		err = res.Validate()
	}
	if err != nil {
		if err != context.Canceled {
			r.c.le.
				WithError(err).
				WithField("via-plugin-id", r.c.conf.GetPluginId()).
				WithField("plugin-id", r.pluginID).
				Warn("failed to fetch plugin")
		}
		return err
	}
	var val plugin_host.FetchPluginValue = res
	_, _ = handler.AddValue(val)
	return nil
}

// _ is a type assertion
var _ directive.Resolver = ((*fetchPluginResolver)(nil))
