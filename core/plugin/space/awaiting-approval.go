package plugin_space

import (
	"context"
	"slices"

	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// WatchAwaitingApproval calls cb with the sorted plugins among pluginIDs whose
// load from this Space waits for the device's approval of a manifest root, then
// again after each change, until ctx ends or the scheduler lookup fails. The
// host scheduler that runs the Space's loads owns that state, as
// PluginSwapAwaitingApproval on each execution. WatchAwaitingApproval only
// reads that state, waking on each change of the scheduler's status container.
func (c *Controller) WatchAwaitingApproval(ctx context.Context, pluginIDs []string, cb func([]string)) error {
	instanceKey := c.GetConfig().GetSpaceId()
	var shown []string
	first := true
	return bldr_plugin.WatchPluginSchedulers(ctx, c.pluginBus(), func(
		_ []bldr_plugin.PluginScheduler,
		snapshots []*bldr_plugin.PluginStatusSnapshot,
	) error {
		ids := awaitingApproval(snapshots, instanceKey, pluginIDs)
		if first || !slices.Equal(ids, shown) {
			first, shown = false, ids
			cb(ids)
		}
		return nil
	})
}

// awaitingApproval returns the sorted plugins among pluginIDs with an
// execution for instanceKey awaiting approval in any of snapshots.
func awaitingApproval(snapshots []*bldr_plugin.PluginStatusSnapshot, instanceKey string, pluginIDs []string) []string {
	// Index the requested plugins.
	requested := make(map[string]struct{}, len(pluginIDs))
	for _, id := range pluginIDs {
		requested[id] = struct{}{}
	}

	// Collect the requested executions of this Space awaiting approval.
	var ids []string
	for _, snapshot := range snapshots {
		for _, exec := range snapshot.GetExecutions() {
			_, ok := requested[exec.GetPluginId()]
			if ok && exec.GetInstanceKey() == instanceKey &&
				exec.GetSwap().GetAwaitingApproval() != nil {
				ids = append(ids, exec.GetPluginId())
			}
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}
