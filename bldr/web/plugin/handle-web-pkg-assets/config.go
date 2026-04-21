package bldr_web_plugin_handle_web_pkg_assets

import (
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bldr_web_plugin "github.com/s4wave/spacewave/bldr/web/plugin"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
)

// ConfigID is the config identifier.
const ConfigID = ControllerID

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig(c, other)
}

// Validate checks the config.
func (c *Config) Validate() error {
	if c.GetWebPluginId() == "" {
		return errors.Wrap(bldr_plugin.ErrEmptyPluginID, "web_plugin_id")
	}
	if c.GetHandlePluginId() == "" {
		return errors.Wrap(bldr_plugin.ErrEmptyPluginID, "handle_plugin_id")
	}
	return nil
}

// ToRequest converts the config into a request.
func (c *Config) ToRequest() *bldr_web_plugin.HandleWebPkgsViaPluginAssetsRequest {
	return &bldr_web_plugin.HandleWebPkgsViaPluginAssetsRequest{
		HandlePluginId: c.GetHandlePluginId(),
		WebPkgsPath:    c.GetWebPkgsPath(),
		WebPkgIdList:   c.GetWebPkgIdList(),
	}
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
