package plugin_host_controller

import (
	"context"
	"sync/atomic"

	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/directive"
)

// resolveLoadPlugin resolves a LoadPlugin directive.
func (c *Controller) resolveLoadPlugin(
	ctx context.Context,
	di directive.Instance,
	dir bldr_plugin.LoadPlugin,
) (directive.Resolver, error) {
	pluginID := dir.LoadPluginID()
	return &loadPluginResolver{c: c, pluginID: pluginID, di: di}, nil
}

// loadPluginResolver resolves LoadPlugin with the controller.
type loadPluginResolver struct {
	// c is the controller
	c *Controller
	// di is the directive instance
	di directive.Instance
	// pluginID is the plugin identifier
	pluginID string
	// relPrev is the previous release func
	// this is in case the resolver is restarted
	relPref atomic.Pointer[func()]
}

// Resolve resolves the values, emitting them to the handler.
func (r *loadPluginResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	handler.ClearValues()

	ref, relRef := r.c.AddPluginReference(r.pluginID)
	var val bldr_plugin.LoadPluginValue = ref
	valID, added := handler.AddValue(val)
	if !added {
		// value rejected
		relRef()
		return nil
	}
	_ = handler.AddValueRemovedCallback(valID, relRef)

	return nil
}

// _ is a type assertion
var _ directive.Resolver = ((*loadPluginResolver)(nil))
