package link_solicit_controller

import (
	"github.com/aperturerobotics/controllerbus/config"
)

// ConfigID is the identifier for the config type.
const ConfigID = ControllerID

// DefaultMaxHashes is the default max hashes per exchange.
const DefaultMaxHashes = 256

// GetConfigID returns the config identifier.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks equality between two configs.
func (c *Config) EqualsConfig(c2 config.Config) bool {
	return config.EqualsConfig[*Config](c, c2)
}

// GetMaxHashesOrDefault returns the max hashes or the default.
func (c *Config) GetMaxHashesOrDefault() uint32 {
	if mh := c.GetMaxHashes(); mh != 0 {
		return mh
	}
	return DefaultMaxHashes
}

// Validate validates the configuration.
func (c *Config) Validate() error { return nil }
