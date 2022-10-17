package plugin_assets_http

import (
	"github.com/aperturerobotics/controllerbus/config"
)

// ConfigID is the config identifier.
const ConfigID = ControllerID

// NewConfig constructs a new config.
func NewConfig(servePath, fsPath string) *Config {
	return &Config{ServePath: servePath, FsPath: fsPath}
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	return nil
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	ot, ok := other.(*Config)
	if !ok {
		return false
	}
	return ot.EqualVT(c)
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
