//go:build !js

package s4wave_core_e2e

import (
	"context"
	"slices"

	manifest_fetch_plugin "github.com/s4wave/spacewave/bldr/manifest/fetch/plugin"
	plugin_host_default "github.com/s4wave/spacewave/bldr/plugin/host/default"
	plugin_host_scheduler "github.com/s4wave/spacewave/bldr/plugin/host/scheduler"
	"github.com/s4wave/spacewave/bldr/testbed"
	web_plugin_compiler "github.com/s4wave/spacewave/bldr/web/plugin/compiler"
	s4wave_testbed "github.com/s4wave/spacewave/sdk/testbed"
	"github.com/sirupsen/logrus"
)

// e2ePluginID is the plugin that runs the TypeScript E2E fixture.
const e2ePluginID = "spacewave-e2e"

// NewNativeTestbed builds the native E2E host with QuickJS reserved for the
// TypeScript fixture and frontend plugins. Go plugins run as native processes.
//
// startPlugins are the run's start plugins. They form the host set, as a
// distribution's startup plugins do, except the E2E fixture plugin: it stays
// outside like any added plugin and reaches the testbed's
// TestbedResourceService only by that service's ID.
func NewNativeTestbed(ctx context.Context, le *logrus.Entry, startPlugins []string) (*testbed.Testbed, error) {
	// Start the native plugin scheduler and retain its testbed.
	hostPluginIDs := slices.DeleteFunc(slices.Clone(startPlugins), func(id string) bool {
		return id == e2ePluginID
	})
	tb, err := testbed.BuildTestbedWithSchedulerConfig(ctx, le, func(
		engineID, objectKey, volumeID, peerID string,
	) *plugin_host_scheduler.Config {
		conf := plugin_host_default.NewNativeDesktopSchedulerConfig(
			"", engineID, objectKey, volumeID, peerID, true, false, false,
			[]string{e2ePluginID, "spacewave-web", "spacewave-app"},
		)
		conf.HostExportPluginIds = hostPluginIDs
		conf.AllowedHostServiceIds = []string{s4wave_testbed.SRPCTestbedResourceServiceServiceID}
		return conf
	})
	if err != nil {
		return nil, err
	}

	// Resolve frontend builds and the core plugin's host manifest fetch config.
	b := tb.GetBus()
	tb.AddFactory(web_plugin_compiler.NewFactory(b))
	tb.AddFactory(manifest_fetch_plugin.NewFactory(b))
	return tb, nil
}
