//go:build !js && !wasip1

package volume_s4db

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
)

// ConfigID is the id attached to the config objects.
var ConfigID = ControllerID

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.GetPath() == "" {
		return errors.New("path required")
	}
	return c.GetKvKeyOpts().Validate()
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ControllerID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig[*Config](c, other)
}

// _ is a type assertion
var _ config.Config = (*Config)(nil)
