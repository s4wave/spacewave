package auth_derive

import (
	"github.com/aperturerobotics/controllerbus/config"
)

// ConfigID identifies the config.
const ConfigID = "auth/derive"

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// Validate validates the configuration.
// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	return nil
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	_, ok := other.(*Config)
	return ok
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
