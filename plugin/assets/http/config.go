package plugin_assets_http

import (
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/config"
)

// ConfigID is the config identifier.
const ConfigID = ControllerID

// NewConfig constructs a new config.
//
// pluginID can be empty
func NewConfig(servePath, fsPath, pluginID string) *Config {
	return &Config{ServePath: servePath, FsPath: fsPath, PluginId: pluginID}
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if err := bldr_plugin.ValidatePluginID(c.GetPluginId(), true); err != nil {
		return err
	}
	return nil
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig(c, other)
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
