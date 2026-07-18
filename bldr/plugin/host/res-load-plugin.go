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

	stateCtr := ref.GetPluginLoadStateCtr()
	var current bldr_plugin.PluginLoadState
	for {
		next, err := stateCtr.WaitValueChange(ctx, current, nil)
		_ = handler.ClearValues()
		if err != nil {
			return err
		}

		current = next
		if running := next.GetRunningPlugin(); running != nil {
			_, _ = handler.AddValue(running)
			handler.MarkIdle(true)
			continue
		}
		if next.GetInitialCapabilityRegistrationState() == bldr_plugin.InitialCapabilityRegistrationFailed {
			handler.MarkIdle(true)
			continue
		}
		handler.MarkIdle(false)
	}
}

// _ is a type assertion
var _ directive.Resolver = ((*LoadPluginResolver)(nil))
