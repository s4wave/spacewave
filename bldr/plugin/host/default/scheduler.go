package plugin_host_default

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	plugin_host_scheduler "github.com/s4wave/spacewave/bldr/plugin/host/scheduler"
)

// StartPluginScheduler starts the plugin host scheduler on the controller bus.
func StartPluginScheduler(
	ctx context.Context,
	b bus.Bus,
	engineID,
	pluginHostObjectKey,
	volID,
	peerID string,
	watchFetchManifest,
	disableStoreManifest,
	disableCopyManifest bool,
) (sched *plugin_host_scheduler.Controller, rel func(), err error) {
	schedConf := NewSchedulerConfig(
		engineID,
		pluginHostObjectKey,
		volID,
		peerID,
		watchFetchManifest,
		disableStoreManifest,
		disableCopyManifest,
	)
	schedCtrl, _, schedCtrlRef, err := loader.WaitExecControllerRunningTyped[*plugin_host_scheduler.Controller](
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(schedConf),
		nil,
	)
	if err != nil {
		return nil, nil, err
	}
	return schedCtrl, schedCtrlRef.Release, nil
}

// NewSchedulerConfig builds the default plugin scheduler config.
func NewSchedulerConfig(
	engineID,
	pluginHostObjectKey,
	volID,
	peerID string,
	watchFetchManifest,
	disableStoreManifest,
	disableCopyManifest bool,
) *plugin_host_scheduler.Config {
	schedConf := plugin_host_scheduler.NewConfig(
		engineID,
		pluginHostObjectKey,
		volID,
		peerID,
		watchFetchManifest,
		disableStoreManifest,
		disableCopyManifest,
	)
	schedConf.PlatformSelectionPolicies = plugin_host_scheduler.SpacewaveDefaultPlatformSelectionPolicies()
	return schedConf
}

// AllowBrowserNativePluginIDs extends the browser-native host allow-list on a
// scheduler config. Devtool browser startup uses this for explicit external
// startup plugins while keeping the production Spacewave defaults unchanged.
func AllowBrowserNativePluginIDs(conf *plugin_host_scheduler.Config, pluginIDs []string) {
	if conf == nil || len(pluginIDs) == 0 {
		return
	}
	for _, policy := range conf.GetPlatformSelectionPolicies() {
		if policy.GetPlatformId() != plugin_host_scheduler.WebJSWASMPlatformID {
			continue
		}
		for _, pluginID := range pluginIDs {
			if pluginID == "" || slices.Contains(policy.AllowedPluginIds, pluginID) {
				continue
			}
			policy.AllowedPluginIds = append(policy.AllowedPluginIds, pluginID)
		}
		return
	}
	allowed := make([]string, 0, len(pluginIDs))
	for _, pluginID := range pluginIDs {
		if pluginID != "" && !slices.Contains(allowed, pluginID) {
			allowed = append(allowed, pluginID)
		}
	}
	if len(allowed) == 0 {
		return
	}
	conf.PlatformSelectionPolicies = append(conf.PlatformSelectionPolicies, &plugin_host_scheduler.PlatformSelectionPolicy{
		PlatformId:       plugin_host_scheduler.WebJSWASMPlatformID,
		AllowedPluginIds: allowed,
	})
}
