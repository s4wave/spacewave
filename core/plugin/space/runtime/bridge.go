package plugin_space_runtime

import (
	"slices"

	"github.com/aperturerobotics/controllerbus/directive"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host_root "github.com/s4wave/spacewave/bldr/plugin/host/root"
	plugin_space "github.com/s4wave/spacewave/core/plugin/space"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
)

// bridgeFilter forwards generation lookups that conf's Space may resolve on the
// parent: its app plugin loads, its own World engine and operations, its plugin
// volumes, plugin host roots, and root object type registrations.
func bridgeFilter(conf *plugin_space.Config, appPluginIDs []string) func(directive.Instance) (bool, error) {
	// Collect the volumes the Space plugin scheduler and process bindings open.
	volumeIDs := []string{bldr_plugin.PluginVolumeID}
	if volumeID := conf.GetVolumeId(); volumeID != "" {
		volumeIDs = append(volumeIDs, volumeID)
	}

	// Filter each child directive against the Space's engine and volumes.
	engineID := conf.GetEngineId()
	return func(inst directive.Instance) (bool, error) {
		return bridgeDirective(inst.GetDirective(), engineID, volumeIDs, appPluginIDs), nil
	}
}

// bridgeDirective reports whether dir resolves on the parent bus. World engine
// and operation lookups forward only for engineID, so a Space plugin cannot
// reach another Space's World; an empty engine ID would match any engine and
// stays inside. Volume lookups forward only for volumeIDs. Space plugin loads,
// manifests, and RPC services stay inside the generation.
func bridgeDirective(dir directive.Directive, engineID string, volumeIDs, appPluginIDs []string) bool {
	switch d := dir.(type) {
	case bldr_plugin.LoadPlugin:
		return slices.Contains(appPluginIDs, d.LoadPluginID())
	case world.LookupWorldEngine:
		return engineID != "" && d.LookupWorldEngineID() == engineID
	case world.LookupWorldOp:
		return engineID != "" && d.LookupWorldOpEngineID() == engineID
	case volume.LookupVolume:
		return slices.Contains(volumeIDs, d.LookupVolumeID())
	case volume.BuildObjectStoreAPI:
		return slices.Contains(volumeIDs, d.BuildObjectStoreAPIVolumeID())
	case objecttype.LookupObjectType, plugin_host_root.LookupRoot:
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
