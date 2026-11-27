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

// bindingKey returns the logical instance a load with instanceKey runs: the
// load's own key, or the scheduler's configured key when the load supplies
// none. The key carries no manifest root or generation, so a plugin's data
// stays put across versions.
func (c *Controller) bindingKey(instanceKey string) string {
	if instanceKey == "" {
		return c.conf.GetInstanceKey()
	}
	return instanceKey
}

// newPluginVolume returns the volume to serve to one execution of a plugin,
// given the logical instance it runs (see bindingKey).
//
// Invariant: a plugin's volume handle reaches only the stores of the instance
// being run. No plugin other than the one that owns the approvals can open the
// store that holds them. A keyed instance, such as a Space's plugin on its
// Space's scheduler or loaded with the Space's key on the app's scheduler,
// gets a view under its instance and plugin, whichever scheduler runs it. An
// instance with the empty key is one of the app's own plugins and still gets
// the host volume whole, so the second half holds only for keyed instances
// until the host identifies the approvals store's owner.
func newPluginVolume(hostVol volume.Volume, bindingKey, pluginID string) volume.Volume {
	if bindingKey == "" {
		return hostVol
	}
	return volume_scoped.NewVolume(hostVol, pluginVolumePrefix(bindingKey, pluginID))
}
