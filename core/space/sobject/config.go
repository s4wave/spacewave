package space_sobject

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
)

// ConfigID is the config identifier.
const ConfigID = ControllerID

// Validate validates the configuration.
func (c *Config) Validate() error {
	if err := c.GetProcessOpsBackoff().Validate(true); err != nil {
		return errors.Wrap(err, "process_ops_backoff")
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
