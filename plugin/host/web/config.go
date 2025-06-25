package plugin_host_web

import (
	web_runtime "github.com/aperturerobotics/bldr/web/runtime"
	"github.com/aperturerobotics/controllerbus/config"
)

// ConfigID is the config identifier.
const ConfigID = ControllerID

// NewConfig constructs a new controller config.
// Sets the most important fields only.
func NewConfig(webRuntimeID string) *Config {
	return &Config{
		WebRuntimeId: webRuntimeID,
	}
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

// Validate validates the configuration.
// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	if c.GetWebRuntimeId() == "" {
		return web_runtime.ErrEmptyWebRuntimeID
	}
	return nil
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
