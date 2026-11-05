package space_world

import (
	"time"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// NewConfig returns the config of the World engine engineID of the Space ref.
func NewConfig(engineID string, ref *sobject.SharedObjectRef) *Config {
	return &Config{EngineId: engineID, Ref: ref}
}

// Validate checks the config.
func (c *Config) Validate() error {
	if err := c.GetRef().Validate(); err != nil {
		return errors.Wrap(err, "ref")
	}
	if c.GetInitWorldOp() != nil {
		if err := c.GetInitWorldOp().Validate(); err != nil {
			return errors.Wrap(err, "init_world_op")
		}
	}
	if _, err := c.ParseReaderPinDelay(); err != nil {
		return errors.Wrap(err, "reader_pin_delay")
	}
	return nil
}

// ParseReaderPinDelay returns the reader pin delay, or nil for the engine's
// default.
func (c *Config) ParseReaderPinDelay() (*time.Duration, error) {
	// Use the default when unset.
	if c.GetReaderPinDelay() == "" {
		return nil, nil
	}

	// Parse it, refusing a negative delay.
	delay, err := time.ParseDuration(c.GetReaderPinDelay())
	if err != nil {
		return nil, err
	}
	if delay < 0 {
		return nil, errors.New("negative delay")
	}
	return &delay, nil
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig[*Config](c, other)
}

// _ is a type assertion
var _ config.Config = (*Config)(nil)
