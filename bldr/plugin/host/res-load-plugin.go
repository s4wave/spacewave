package plugin_host

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// LoadPluginResolver resolves LoadPlugin with the controller.
type LoadPluginResolver struct {
	// c is the controller
	c PluginHostScheduler
	// dir is the directive being resolved.
	dir bldr_plugin.LoadPlugin
	// instanceKey is the effective instance key for instanced plugins.
	instanceKey string
}

// NewLoadPluginResolver constructs a new LoadPluginResolver. instanceKey
// replaces the directive's instance key, which may be empty.
func NewLoadPluginResolver(c PluginHostScheduler, dir bldr_plugin.LoadPlugin, instanceKey string) *LoadPluginResolver {
	return &LoadPluginResolver{c: c, dir: dir, instanceKey: instanceKey}
}

// Resolve resolves the values, emitting them to the handler.
func (r *LoadPluginResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	// Add the plugin reference matching the requested manifests, manifest
	// root, served assets, or plain plugin ID, releasing it when resolution ends.
	pluginID, manifestRoot := r.dir.LoadPluginID(), r.dir.LoadPluginManifestRoot()
	var ref bldr_plugin.RunningPluginRef
	var relRef func()
	switch {
	case r.dir.LoadPluginServeAssets():
		ref, relRef = r.c.AddAssetsPluginReference(pluginID, r.instanceKey, manifestRoot)
	case len(r.dir.LoadPluginManifests()) != 0:
		ref, relRef = r.c.AddSelectedPluginReference(pluginID, r.instanceKey, r.dir.LoadPluginManifests()...)
	case manifestRoot != "":
		ref, relRef = r.c.AddPinnedPluginReference(pluginID, r.instanceKey, manifestRoot)
	default:
		ref, relRef = r.c.AddPluginReference(pluginID, r.instanceKey)
	}
	defer relRef()

	// Stream each plugin load state change to the handler until the context
	// is canceled or the load fails.
	stateCtr := ref.GetPluginLoadStateCtr()
	var current bldr_plugin.PluginLoadState
	for {
		next, err := stateCtr.WaitValueChange(ctx, current, nil)
		_ = handler.ClearValues()
		if err != nil {
			return err
		}
		current = next
		if r.dir.LoadPluginServeAssets() && next.GetManifestRoot() != "" {
			_, _ = handler.AddValue(next.GetManifestRoot())
			handler.MarkIdle(true)
			continue
		}
		if running := next.GetRunningPlugin(); running != nil {
			_, _ = handler.AddValue(running)
			handler.MarkIdle(true)
			continue
		}
		if next.GetInitialCapabilityRegistrationState() == bldr_plugin.InitialCapabilityRegistrationFailed {
			if manifestRoot != "" {
				return errors.Errorf("plugin %s: exact manifest %s is unavailable", pluginID, manifestRoot)
			}
			handler.MarkIdle(true)
			continue
		}
		handler.MarkIdle(false)
	}
}

// _ is a type assertion
var _ directive.Resolver = (*LoadPluginResolver)(nil)
