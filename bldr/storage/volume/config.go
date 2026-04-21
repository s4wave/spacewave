package storage_volume

import (
	"errors"

	"github.com/aperturerobotics/controllerbus/config"
)

// ConfigID is the id attached to the config objects.
var ConfigID = ControllerID

// Validate validates the configuration.
// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	if id := c.GetStorageId(); id == "" {
		return errors.New("storage_id cannot be empty")
	}

	if id := c.GetStorageVolumeId(); id == "" {
		return errors.New("storage_volume_id cannot be empty")
	}

	if err := c.GetVolumeConfig().Validate(); err != nil {
		return err
	}

	return nil
}

// GetConfigID returns the unique string for this configuration type.
// This string is stored with the encoded config.
func (c *Config) GetConfigID() string {
	return ControllerID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig[*Config](c, other)
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
