package plugin_space_runtime

import (
	"slices"

	"github.com/aperturerobotics/controllerbus/directive"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host_root "github.com/s4wave/spacewave/bldr/plugin/host/root"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
)

// appPluginIDs are the plugins the running app ships. Space plugins load them
// from the app: a generation forwards their loads to the parent bus and its
// scheduler leaves them alone, so a Space neither lists nor requests them.
var appPluginIDs = []string{"spacewave-core", "spacewave-web", "spacewave-app", "web"}

// bridgeFilter forwards the parent infrastructure lookups into a generation.
func bridgeFilter(inst directive.Instance) (bool, error) {
	return bridgeDirective(inst.GetDirective()), nil
}

// bridgeDirective reports whether dir resolves on the parent bus: app plugin
// loads and infrastructure lookups. Space plugin loads, manifests, hosts, and
// RPC services stay inside the generation.
func bridgeDirective(dir directive.Directive) bool {
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

// schedulerLookupFilter forwards LookupPluginScheduler from the parent bus into
// a generation so session status sees its scheduler.
func schedulerLookupFilter(inst directive.Instance) (bool, error) {
	_, ok := inst.GetDirective().(bldr_plugin.LookupPluginScheduler)
	return ok, nil
}
