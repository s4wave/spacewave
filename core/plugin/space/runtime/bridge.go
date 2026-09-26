package plugin_space_runtime

import (
	"github.com/aperturerobotics/controllerbus/directive"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host_root "github.com/s4wave/spacewave/bldr/plugin/host/root"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
)

// bridgeFilter forwards the parent infrastructure lookups into a generation.
func bridgeFilter(inst directive.Instance) (bool, error) {
	return bridgeDirective(inst.GetDirective()), nil
}

// bridgeDirective reports whether dir resolves on the parent bus. Plugin loads,
// manifests, hosts, and RPC services stay inside the generation.
func bridgeDirective(dir directive.Directive) bool {
	switch dir.(type) {
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
