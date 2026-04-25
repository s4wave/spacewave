package plugin_space

import (
	"errors"

	"github.com/aperturerobotics/controllerbus/config"
)

// ConfigID is the config identifier.
const ConfigID = ControllerID

// ErrEmptySpaceID is returned when space_id is empty.
var ErrEmptySpaceID = errors.New("space_id must be specified")

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.GetSpaceId() == "" {
		return ErrEmptySpaceID
	}
	return nil
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig(c, other)
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
