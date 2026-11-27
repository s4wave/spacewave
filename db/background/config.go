package background

import (
	"github.com/aperturerobotics/controllerbus/config"
)

// ConfigID is the identifier for the config type.
const ConfigID = ControllerID

// defaultRunSlots is the default number of jobs that run at once.
const defaultRunSlots = 1

// Validate validates the configuration.
func (c *Config) Validate() error {
	return c.GetRetryBackoff().Validate(true)
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig[*Config](c, other)
}

// GetRunSlotsOrDefault returns the run slots or the default number of slots.
func (c *Config) GetRunSlotsOrDefault() uint32 {
	return max(c.GetRunSlots(), defaultRunSlots)
}

// _ is a type assertion
var _ config.Config = (*Config)(nil)
