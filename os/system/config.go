package os_system

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
)

// ConfigID is the string used to identify this config object.
const ConfigID = NodeTypeID

// Validate requires the World engine and the release.
func (c *Config) Validate() error {
	if c.GetEngineId() == "" {
		return world.ErrEmptyEngineID
	}
	if c.GetObjectKey() == "" {
		return errors.Wrap(world.ErrEmptyObjectKey, "release")
	}
	return nil
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the other config is equal.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig(c, other)
}

// _ is a type assertion
var _ config.Config = (*Config)(nil)
