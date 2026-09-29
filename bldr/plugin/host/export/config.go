package plugin_host_export

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
)

// ControllerID is the host export proxy controller ID.
const ControllerID = "bldr/plugin/host/export"

// ConfigID is the config identifier.
const ConfigID = ControllerID

// Version is the version of this controller.
var Version = controller.MustParseVersion("0.0.1")

// GetConfigID returns the unique string for this configuration type.
// This string is stored with the encoded config.
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

// Validate validates the configuration.
func (c *Config) Validate() error {
	return nil
}

// _ is a type assertion
var _ config.Config = (*Config)(nil)
