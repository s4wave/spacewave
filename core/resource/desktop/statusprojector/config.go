package statusprojector

import (
	"github.com/aperturerobotics/controllerbus/config"
)

// ConfigID is the config identifier.
const ConfigID = ControllerID

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	ot, ok := other.(*Config)
	if !ok {
		return false
	}
	return c.EqualVT(ot)
}

// Validate checks the config.
func (c *Config) Validate() error {
	return nil
}

// ResolvedWebRuntimeID returns the configured web runtime id.
func (c *Config) ResolvedWebRuntimeID() string {
	return c.GetWebRuntimeId()
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
