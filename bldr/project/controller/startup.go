//go:build !js

package bldr_project_controller

import (
	"context"

	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host_scheduler "github.com/s4wave/spacewave/bldr/plugin/host/scheduler"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
)

// executeStartup executes the configured Startup part of the project config.
func (c *Controller) executeStartup(ctx context.Context, conf *bldr_project.StartConfig) error {
	loadPluginIDs := conf.GetPlugins()
	if len(loadPluginIDs) == 0 {
		return nil
	}

	for _, pluginID := range loadPluginIDs {
		c.le.WithField("plugin-id", pluginID).Info("loading startup plugin")
		_, plugRef, err := c.bus.AddDirective(bldr_plugin.NewLoadPlugin(pluginID), nil)
		if err != nil {
			return err
		}
		defer plugRef.Release()
	}

	scheduler, err := plugin_host_scheduler.WaitControllerOnBus(ctx, c.bus)
	if err != nil {
		return err
	}
	if err := scheduler.WaitPluginsRunning(ctx, loadPluginIDs); err != nil {
		return err
	}

	// wait for context cancel to release plugin refs
	<-ctx.Done()
	return context.Canceled
}
