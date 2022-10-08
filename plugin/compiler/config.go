package plugin_compiler

import (
	builder "github.com/aperturerobotics/bldr/plugin/builder"
	"github.com/aperturerobotics/controllerbus/config"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/pkg/errors"
	"golang.org/x/mod/module"
)

// ConfigID is the config identifier.
const ConfigID = ControllerID

// NewConfig constructs a new config.
func NewConfig() *Config {
	return &Config{}
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if err := c.GetPluginBuilderConfig().Validate(); err != nil {
		return err
	}
	if err := configset_proto.ConfigSetMap(c.GetConfigSet()).Validate(); err != nil {
		return errors.Wrap(err, "config_set")
	}
	for i, impPath := range c.GetGoPackages() {
		if err := module.CheckImportPath(impPath); err != nil {
			return errors.Wrapf(err, "go_packages[%d]: invalid import path", i)
		}
	}
	return nil
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	ot, ok := other.(*Config)
	if !ok {
		return false
	}
	return ot.EqualVT(c)
}

// SetPluginBuilderConfig configures the common plugin builder settings.
func (c *Config) SetPluginBuilderConfig(conf *builder.PluginBuilderConfig) {
	c.PluginBuilderConfig = conf
}

// _ is a type assertion
var _ builder.ControllerConfig = ((*Config)(nil))
