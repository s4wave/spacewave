package bldr_project_controller

import (
	"context"

	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	bldr_project "github.com/aperturerobotics/bldr/project"
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

	// wait for context cancel to release plugin refs
	<-ctx.Done()
	return context.Canceled
}
