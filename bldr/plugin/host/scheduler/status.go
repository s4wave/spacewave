package plugin_host_scheduler

import (
	"slices"

	"github.com/aperturerobotics/util/ccontainer"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// PluginStatusSnapshot describes the scheduler's current plugin instances.
type PluginStatusSnapshot struct {
	Plugins []*bldr_plugin.PluginStatus
}

// GetPluginStatusCtr returns the scheduler's live plugin-status snapshot.
func (c *Controller) GetPluginStatusCtr() ccontainer.Watchable[*PluginStatusSnapshot] {
	return c.pluginStatusCtr
}

func (c *Controller) setPluginStatus(
	pluginID,
	instanceKey string,
	state bldr_plugin.PluginState,
) {
	key := pluginInstanceKey(pluginID, instanceKey)
	c.pluginStatusMtx.Lock()
	defer c.pluginStatusMtx.Unlock()
	if state == bldr_plugin.PluginState_PluginState_UNKNOWN {
		delete(c.pluginStatus, key)
	} else {
		c.pluginStatus[key] = &bldr_plugin.PluginStatus{
			PluginId:    pluginID,
			InstanceKey: instanceKey,
			Running:     state == bldr_plugin.PluginState_PluginState_RUNNING,
			State:       state,
		}
	}
	c.pluginStatusCtr.SetValue(c.buildPluginStatusSnapshotLocked())
}

func (c *Controller) buildPluginStatusSnapshotLocked() *PluginStatusSnapshot {
	plugins := make([]*bldr_plugin.PluginStatus, 0, len(c.pluginStatus))
	for _, plugin := range c.pluginStatus {
		plugins = append(plugins, &bldr_plugin.PluginStatus{
			PluginId:    plugin.PluginId,
			InstanceKey: plugin.InstanceKey,
			Running:     plugin.Running,
			State:       plugin.State,
		})
	}
	slices.SortFunc(plugins, func(a, b *bldr_plugin.PluginStatus) int {
		if a.PluginId < b.PluginId {
			return -1
		}
		if a.PluginId > b.PluginId {
			return 1
		}
		if a.InstanceKey < b.InstanceKey {
			return -1
		}
		if a.InstanceKey > b.InstanceKey {
			return 1
		}
		return 0
	})
	return &PluginStatusSnapshot{Plugins: plugins}
}

func pluginStatusSnapshotEqual(a, b *PluginStatusSnapshot) bool {
	if a == nil || b == nil {
		return a == b
	}
	if len(a.Plugins) != len(b.Plugins) {
		return false
	}
	for i, ap := range a.Plugins {
		bp := b.Plugins[i]
		if ap.PluginId != bp.PluginId ||
			ap.InstanceKey != bp.InstanceKey ||
			ap.Running != bp.Running ||
			ap.State != bp.State {
			return false
		}
	}
	return true
}
