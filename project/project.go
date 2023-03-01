package bldr_project

import (
	"github.com/aperturerobotics/bifrost/util/labels"
	"github.com/aperturerobotics/bldr/plugin"
	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	jsonpb "google.golang.org/protobuf/encoding/protojson"
)

// UnmarshalProjectConfig unmarshals a project config from json or yaml.
func UnmarshalProjectConfig(data []byte, conf *ProjectConfig) error {
	jdata, err := yaml.YAMLToJSON(data)
	if err != nil {
		return err
	}

	return jsonpb.Unmarshal(jdata, conf)
}

// ValidateProjectID validates a project identifier.
func ValidateProjectID(id string) error {
	if id == "" {
		return ErrEmptyProjectID
	}
	if err := labels.ValidateDNSLabel(id); err != nil {
		return errors.Wrap(err, "project id")
	}
	return nil
}

// Validate validates the project configuration.
func (c *ProjectConfig) Validate() error {
	if err := ValidateProjectID(c.GetId()); err != nil {
		return err
	}
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

// GetEmbedPluginsList returns the list of plugins to embed in dist.
func (c *ProjectConfig) GetEmbedPluginsList() []string {
	if embedPlugins := c.GetDist().GetEmbedPlugins(); len(embedPlugins) != 0 {
		return embedPlugins
	}
	return c.GetStart().GetPlugins()
}

// Validate validates the start configuration.
func (c *StartConfig) Validate() error {
	for _, pluginID := range c.GetPlugins() {
		if err := plugin.ValidatePluginID(pluginID); err != nil {
			return errors.Wrapf(err, "plugins[%s]: invalid plugin id", pluginID)
		}
	}
	return nil
}
