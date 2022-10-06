package bldr_project

import (
	"context"

	"github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/pkg/errors"
)

// Validate validates the project configuration.
func (c *ProjectConfig) Validate() error {
	if err := c.GetStart().Validate(); err != nil {
		return errors.Wrap(err, "start")
	}
	for pluginID, pluginConf := range c.GetPlugins() {
		if err := plugin.ValidatePluginID(pluginID); err != nil {
			return errors.Wrap(err, "plugins: invalid plugin id")
		}
		if err := pluginConf.Validate(); err != nil {
			return errors.Wrapf(err, "plugins[%s]: config invalid", pluginID)
		}
	}
	return nil
}

// Validate validates the start configuration.
func (c *StartConfig) Validate() error {
	for _, pluginID := range c.GetLoadPluginIds() {
		if err := plugin.ValidatePluginID(pluginID); err != nil {
			return errors.Wrap(err, "load_plugin_ids: invalid plugin id")
		}
	}
	csm := configset_proto.ConfigSetMap(c.GetConfigSet())
	if err := csm.Validate(); err != nil {
		return errors.Wrap(err, "config_set")
	}
	return nil
}

// ResolveConfigSet parses and resolves the config set yaml.
func (c *StartConfig) ResolveConfigSet(ctx context.Context, b bus.Bus) (configset.ConfigSet, error) {
	csm := configset_proto.ConfigSetMap(c.GetConfigSet())
	return csm.Resolve(ctx, b)
}
