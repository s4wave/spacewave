package bldr_project

import (
	"github.com/aperturerobotics/bifrost/util/labels"
	manifest "github.com/aperturerobotics/bldr/manifest"
	plugin "github.com/aperturerobotics/bldr/plugin"
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
	for manifestID, manifestConf := range c.GetManifests() {
		if err := manifest.ValidateManifestID(manifestID, false); err != nil {
			return errors.Wrap(err, "manifests: invalid manifest id")
		}
		if err := manifestConf.Validate(); err != nil {
			return errors.Wrapf(err, "manifests[%s]: config invalid", manifestID)
		}
	}
	return nil
}

// Validate validates the start configuration.
func (c *StartConfig) Validate() error {
	for _, pluginID := range c.GetPlugins() {
		if err := plugin.ValidatePluginID(pluginID, false); err != nil {
			return errors.Wrapf(err, "plugins[%s]: invalid plugin id", pluginID)
		}
	}
	return nil
}

// Validate validates the plugin config.
func (c *ManifestConfig) Validate() error {
	if err := c.GetBuilder().Validate(); err != nil {
		return errors.Wrap(err, "builder")
	}
	return nil
}
