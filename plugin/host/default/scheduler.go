package plugin_host_default

import (
	"context"

	plugin_host_scheduler "github.com/aperturerobotics/bldr/plugin/host/scheduler"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
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
	schedConf := plugin_host_scheduler.NewConfig(
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
