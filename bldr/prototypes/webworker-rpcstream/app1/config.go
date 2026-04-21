package app1

import "github.com/aperturerobotics/controllerbus/config"

// ConfigID is the config identifier.
const ConfigID = ControllerID

// Validate validates the configuration.
func (c *Config) Validate() error {
	return nil
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	_, ok := other.(*Config)
	return ok
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
