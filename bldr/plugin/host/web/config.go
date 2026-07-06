package plugin_host_web

import (
	"github.com/aperturerobotics/controllerbus/config"
	web_runtime "github.com/s4wave/spacewave/bldr/web/runtime"
)

// ConfigID is the config identifier.
const ConfigID = ControllerID

// NewConfig constructs a new controller config.
// Sets the most important fields only.
func NewConfig(webRuntimeID, platformID string) *Config {
	return &Config{
		WebRuntimeId: webRuntimeID,
		PlatformId:   platformID,
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
func (c *Config) Validate() error {
	if c.GetWebRuntimeId() == "" {
		return web_runtime.ErrEmptyWebRuntimeID
	}
	_, err := parseWebHostPlatform(c.GetPlatformId())
	return err
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
