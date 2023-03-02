package plugin_web

import (
	builder "github.com/aperturerobotics/bldr/plugin/builder"
	"github.com/aperturerobotics/controllerbus/config"
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

// SetDisableWatch sets the disable watch field, if applicable.
func (c *Config) SetDisableWatch(disable bool) {
	// no-op
}

// _ is a type assertion
var _ builder.ControllerConfig = ((*Config)(nil))
