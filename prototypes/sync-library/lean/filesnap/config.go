package volume_filesnap

import (
	"github.com/aperturerobotics/controllerbus/config"
)

// ConfigID is the id attached to the config objects.
var ConfigID = ControllerID

// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	if c.GetPath() == "" {
		return errPathEmpty
	}
	return nil
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string { return ConfigID }

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig[*Config](c, other)
}

// _ is a type assertion
var _ config.Config = (*Config)(nil)
