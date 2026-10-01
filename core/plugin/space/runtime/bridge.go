package plugin_space_runtime

import (
	"slices"

	"github.com/aperturerobotics/controllerbus/directive"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host_root "github.com/s4wave/spacewave/bldr/plugin/host/root"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
)

// bridgeFilter forwards the parent infrastructure lookups into a generation.
func bridgeFilter(appPluginIDs []string) func(directive.Instance) (bool, error) {
	return func(inst directive.Instance) (bool, error) {
		return bridgeDirective(inst.GetDirective(), appPluginIDs), nil
	}
}

// bridgeDirective reports whether dir resolves on the parent bus: app plugin
// loads and live infrastructure lookups. Space plugin loads, manifests, and
// RPC services stay inside the generation.
func bridgeDirective(dir directive.Directive, appPluginIDs []string) bool {
	switch d := dir.(type) {
	case bldr_plugin.LoadPlugin:
		return slices.Contains(appPluginIDs, d.LoadPluginID())
	case world.LookupWorldEngine, world.LookupWorldOp,
		volume.LookupVolume, volume.BuildObjectStoreAPI,
		plugin_host_root.LookupRoot:
		return true
	default:
		return false
	}
}

// parentFilter forwards parent lookups the generation answers into it.
func parentFilter(instanceKey string, appPluginIDs []string) func(directive.Instance) (bool, error) {
	return func(inst directive.Instance) (bool, error) {
		return parentDirective(inst.GetDirective(), instanceKey, appPluginIDs), nil
	}
}

// parentDirective reports whether dir on the parent bus resolves in the
// generation: LookupPluginScheduler, so session status sees its scheduler, and
// loads of this Space's plugin installation, so the daemon can reach a Space
// plugin that registered a handler. An empty instanceKey forwards no loads.
// App plugin loads stay on the parent.
func parentDirective(dir directive.Directive, instanceKey string, appPluginIDs []string) bool {
	switch d := dir.(type) {
	case bldr_plugin.LookupPluginScheduler:
		return true
	case bldr_plugin.LoadPlugin:
		return instanceKey != "" &&
			d.LoadPluginInstanceKey() == instanceKey &&
			!slices.Contains(appPluginIDs, d.LoadPluginID())
	default:
		return false
	}
}
