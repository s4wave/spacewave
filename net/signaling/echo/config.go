package signaling_echo

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/s4wave/spacewave/net/signaling"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// Validate validates the configuration.
// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	if c.GetSignalingId() == "" {
		return signaling.ErrEmptySignalingID
	}

	return nil
}

// GetConfigID returns the unique string for this configuration type.
// This string is stored with the encoded config.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(c2 config.Config) bool {
	return config.EqualsConfig[*Config](c, c2)
}

var _ config.Config = ((*Config)(nil))
