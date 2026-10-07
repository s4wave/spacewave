package plugin_host_scheduler

import (
	"net/url"

	"github.com/s4wave/spacewave/db/volume"
	volume_scoped "github.com/s4wave/spacewave/db/volume/scoped"
)

// pluginVolumePrefix returns the ID prefix of a Space plugin's volume view.
//
// The components are path-escaped, so no pair of Space and plugin IDs shares a
// prefix with another pair.
func pluginVolumePrefix(instanceKey, pluginID string) string {
	return "plugin-volume/" + url.PathEscape(instanceKey) + "/" + url.PathEscape(pluginID) + "/"
}

// newPluginVolume returns the volume to serve to a plugin.
//
// A scheduler with an instance key is a Space's scheduler: its plugins reach
// only the data under their Space and plugin. Other schedulers run the app's
// own plugins, which use the host volume directly.
func (c *Controller) newPluginVolume(hostVol volume.Volume, pluginID string) volume.Volume {
	instanceKey := c.conf.GetInstanceKey()
	if instanceKey == "" {
		return hostVol
	}
	return volume_scoped.NewVolume(hostVol, pluginVolumePrefix(instanceKey, pluginID))
}
