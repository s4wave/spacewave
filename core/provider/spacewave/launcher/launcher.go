package spacewave_launcher

import (
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// ChannelStable is the default release channel.
const ChannelStable = "stable"

// PluginID is the plugin that runs the launcher controller: its own process on
// desktop and its own worker in the browser.
const PluginID = "spacewave-launcher"

// PluginLauncherServiceID routes Launcher calls from any process to the
// launcher plugin. The controller answers the bare SRPCLauncherServiceID only
// on its own plugin bus.
var PluginLauncherServiceID = bldr_plugin.PluginServiceID(PluginID, SRPCLauncherServiceID)

// ResolvedChannelKey returns the DistConfig channel key with the empty-string
// fallback applied.
func (c *DistConfig) ResolvedChannelKey() string {
	if ch := c.GetChannelKey(); ch != "" {
		return ch
	}
	return ChannelStable
}

// Validate performs basic validation of the config.
func (c *DistConfig) Validate() error {
	if len(c.GetProjectId()) == 0 {
		return errors.New("project id cannot be empty")
	}
	if c.GetRev() == 0 {
		return errors.New("rev cannot be empty")
	}
	if c.GetChannelKey() == "" {
		return errors.New("channel key cannot be empty")
	}
	if err := configset_proto.ConfigSetMap(c.GetLauncherConfigSet()).Validate(); err != nil {
		return errors.Wrap(err, "launcher_config_set")
	}
	return nil
}

// MarshalToJSON marshals the configuration to json.
func (c *DistConfig) MarshalToJSON() ([]byte, error) {
	return c.MarshalJSON()
}
