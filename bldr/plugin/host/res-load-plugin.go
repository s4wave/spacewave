package plugin_host

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// LoadPluginResolver resolves LoadPlugin with the controller.
type LoadPluginResolver struct {
	// c is the controller
	c PluginHostScheduler
	// pluginID is the plugin identifier
	pluginID string
	// instanceKey is the instance key for instanced plugins.
	instanceKey string
}

// NewLoadPluginResolver constructs a new LoadPluginResolver.
func NewLoadPluginResolver(c PluginHostScheduler, pluginID, instanceKey string) *LoadPluginResolver {
	return &LoadPluginResolver{c: c, pluginID: pluginID, instanceKey: instanceKey}
}

// Resolve resolves the values, emitting them to the handler.
func (r *LoadPluginResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	ref, relRef := r.c.AddPluginReference(r.pluginID, r.instanceKey)
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
		if nextVal != nil {
			val := nextVal
			_, _ = handler.AddValue(val)
			handler.MarkIdle(true)
		}
	}
}

// _ is a type assertion
var _ directive.Resolver = ((*LoadPluginResolver)(nil))
