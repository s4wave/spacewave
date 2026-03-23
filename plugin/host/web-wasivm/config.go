//go:build js || wasip1

package plugin_host_web_wasivm

import (
	"github.com/aperturerobotics/controllerbus/config"
)

// ConfigID is the config identifier.
const ConfigID = ControllerID

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig(c, other)
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	return nil
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
