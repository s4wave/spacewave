package plugin_host_controller

import (
	"context"

	"github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/util/keyed"
	"github.com/pkg/errors"
)

// runningPlugin manages a running plugin instance
type runningPlugin struct {
	// c is the controller
	c *Controller
	// pluginID is the plugin id
	pluginID string
	// manifest is the plugin manifest
	manifest *plugin.PluginManifest
}

// newRunningPlugin constructs a new running plugin routine.
func (c *Controller) newRunningPlugin(key string) (keyed.Routine, *runningPlugin) {
	c.rmtx.RLock()
	manifest := c.pluginManifests[key]
	c.rmtx.RUnlock()
	tr := &runningPlugin{
		c:        c,
		pluginID: key,
		manifest: manifest,
	}
	return tr.execute, tr
}

// execute executes the plugin.
func (t *runningPlugin) execute(ctx context.Context) error {
	pluginID, le := t.pluginID, t.c.le
	manifest := t.manifest

	if manifest.GetPluginId() == "" {
		le.Debug("waiting for plugin manifest: %s", pluginID)
		return nil
	}

	le.Debugf("starting plugin: %s", pluginID)
	return errors.New("TODO execute plugin: " + pluginID)
}
