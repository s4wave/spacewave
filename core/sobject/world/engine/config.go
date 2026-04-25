package sobject_world_engine

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// NewConfig constructs a new block world engine config.
func NewConfig(engineID string, ref *sobject.SharedObjectRef) *Config {
	return &Config{
		EngineId: engineID,
		Ref:      ref,
	}
}

// Validate validates the configuration.
// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	if err := c.GetRef().Validate(); err != nil {
		return errors.Wrap(err, "ref")
	}
	if err := c.GetInitWorldOp().Validate(); err != nil {
		return errors.Wrap(err, "init_world_op")
	}
	if err := c.GetProcessOpsBackoff().Validate(true); err != nil {
		return errors.Wrap(err, "process_ops_backoff")
	}

	return nil
}

// GetConfigID returns the unique string for this configuration type.
// This string is stored with the encoded config.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig(c, other)
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
