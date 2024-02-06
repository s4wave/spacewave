package plugin_host_controller

import (
	"context"

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
}

// Resolve resolves the values, emitting them to the handler.
func (r *loadPluginResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	ref, relRef := r.c.AddPluginReference(r.pluginID)
	defer relRef()

	rpCtr := ref.GetRunningPluginCtr()
	var currVal bldr_plugin.RunningPlugin
	for {
		nextVal, err := rpCtr.WaitValueChange(ctx, currVal, nil)
		_ = handler.ClearValues()
		if err != nil {
			return err
		}

		currVal = nextVal
		var val bldr_plugin.LoadPluginValue = nextVal
		_, _ = handler.AddValue(val)
	}
}

// _ is a type assertion
var _ directive.Resolver = ((*loadPluginResolver)(nil))
