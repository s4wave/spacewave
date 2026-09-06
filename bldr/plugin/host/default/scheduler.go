package plugin_host_default

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	plugin_host_scheduler "github.com/s4wave/spacewave/bldr/plugin/host/scheduler"
)

// StartPluginScheduler starts the plugin host scheduler on the controller bus.
func StartPluginScheduler(
	ctx context.Context,
	b bus.Bus,
	instanceKey,
	engineID,
	pluginHostObjectKey,
	volID,
	peerID string,
	watchFetchManifest,
	disableStoreManifest,
	disableCopyManifest bool,
) (sched *plugin_host_scheduler.Controller, rel func(), err error) {
	schedConf := NewSchedulerConfig(
		instanceKey,
		engineID,
		pluginHostObjectKey,
		volID,
		peerID,
		watchFetchManifest,
		disableStoreManifest,
		disableCopyManifest,
	)
	return startPluginSchedulerWithConfig(ctx, b, schedConf)
}

// StartNativeDesktopPluginScheduler starts the native desktop plugin scheduler.
func StartNativeDesktopPluginScheduler(
	ctx context.Context,
	b bus.Bus,
	instanceKey,
	engineID,
	pluginHostObjectKey,
	volID,
	peerID string,
	watchFetchManifest,
	disableStoreManifest,
	disableCopyManifest bool,
	quickJSPluginIDs []string,
) (sched *plugin_host_scheduler.Controller, rel func(), err error) {
	schedConf := NewNativeDesktopSchedulerConfig(
		instanceKey,
		engineID,
		pluginHostObjectKey,
		volID,
		peerID,
		watchFetchManifest,
		disableStoreManifest,
		disableCopyManifest,
		quickJSPluginIDs,
	)
	return startPluginSchedulerWithConfig(ctx, b, schedConf)
}

func startPluginSchedulerWithConfig(
	ctx context.Context,
	b bus.Bus,
	schedConf *plugin_host_scheduler.Config,
) (sched *plugin_host_scheduler.Controller, rel func(), err error) {
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
	instanceKey,
	engineID,
	pluginHostObjectKey,
	volID,
	peerID string,
	watchFetchManifest,
	disableStoreManifest,
	disableCopyManifest bool,
) *plugin_host_scheduler.Config {
	schedConf := plugin_host_scheduler.NewConfig(
		instanceKey,
		engineID,
		pluginHostObjectKey,
		volID,
		peerID,
		watchFetchManifest,
		disableStoreManifest,
		disableCopyManifest,
	)
	schedConf.MaterializerPluginId = materializerPluginID()
	return schedConf
}

// NewNativeDesktopSchedulerConfig builds the native desktop plugin scheduler config.
func NewNativeDesktopSchedulerConfig(
	instanceKey,
	engineID,
	pluginHostObjectKey,
	volID,
	peerID string,
	watchFetchManifest,
	disableStoreManifest,
	disableCopyManifest bool,
	quickJSPluginIDs []string,
) *plugin_host_scheduler.Config {
	schedConf := NewSchedulerConfig(
		instanceKey,
		engineID,
		pluginHostObjectKey,
		volID,
		peerID,
		watchFetchManifest,
		disableStoreManifest,
		disableCopyManifest,
	)
	allowedPluginIDs := slices.Clone(quickJSPluginIDs)
	slices.Sort(allowedPluginIDs)
	allowedPluginIDs = slices.Compact(allowedPluginIDs)
	schedConf.PlatformSelectionPolicies = append(
		schedConf.PlatformSelectionPolicies,
		&plugin_host_scheduler.PlatformSelectionPolicy{
			PlatformId:       bldr_platform.PlatformID_JS,
			AllowedPluginIds: allowedPluginIDs,
		},
	)
	return schedConf
}
