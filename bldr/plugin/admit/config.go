package bldr_plugin_admit

import (
	"github.com/aperturerobotics/controllerbus/config"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// ConfigID is the config identifier.
const ConfigID = ControllerID

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	ot, ok := other.(*Config)
	return ok && c.EqualVT(ot)
}

// Validate checks the config.
func (c *Config) Validate() error {
	return bldr_plugin.ValidatePluginID(c.GetPluginId(), false)
}

// _ is a type assertion
var _ config.Config = (*Config)(nil)
