package bldr_project

import (
	"github.com/aperturerobotics/bldr/plugin"
	"github.com/ghodss/yaml"
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
	if c.GetConfigSetYaml() != "" {
		if _, err := yaml.YAMLToJSON([]byte(c.GetConfigSetYaml())); err != nil {
			return errors.Wrap(err, "config_set_yaml")
		}
	}
	return nil
}
