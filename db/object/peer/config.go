package object_peer

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/s4wave/spacewave/db/object"
)

// ConfigID is the config identifier.
const ConfigID = ControllerID

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.GetObjectStoreId() == "" {
		return object.ErrEmptyObjectStoreId
	}
	if err := c.GetTransformConf().Validate(); err != nil {
		return err
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
