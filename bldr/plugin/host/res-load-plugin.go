package plugin_host

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
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
	// manifestRoot selects immutable execution when nonempty.
	manifestRoot string
	// manifests contains the caller's installation and recovery artifacts.
	manifests []*manifest.ManifestRef
}

// NewLoadPluginResolver constructs a new LoadPluginResolver.
func NewLoadPluginResolver(c PluginHostScheduler, pluginID, instanceKey, manifestRoot string, selected []*manifest.ManifestRef) *LoadPluginResolver {
	return &LoadPluginResolver{c: c, pluginID: pluginID, instanceKey: instanceKey, manifestRoot: manifestRoot, manifests: selected}
}

// Resolve resolves the values, emitting them to the handler.
func (r *LoadPluginResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	var ref bldr_plugin.RunningPluginRef
	var relRef func()
	if len(r.manifests) != 0 {
		ref, relRef = r.c.AddSelectedPluginReference(r.pluginID, r.instanceKey, r.manifests...)
	} else if r.manifestRoot == "" {
		ref, relRef = r.c.AddPluginReference(r.pluginID, r.instanceKey)
	} else {
		ref, relRef = r.c.AddPinnedPluginReference(r.pluginID, r.instanceKey, r.manifestRoot)
	}
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
			if r.manifestRoot != "" {
				return errors.Errorf("plugin %s: exact manifest %s is unavailable", r.pluginID, r.manifestRoot)
			}
			handler.MarkIdle(true)
			continue
		}
		handler.MarkIdle(false)
	}
}

// _ is a type assertion
var _ directive.Resolver = (*LoadPluginResolver)(nil)
