package bldr_web_plugin_handle_rpc

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	handle_rpc_viaplugin "github.com/s4wave/spacewave/bldr/plugin/forward-rpc-service"
	bldr_web_plugin "github.com/s4wave/spacewave/bldr/web/plugin"
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
	if c.GetWebPluginId() == "" {
		return errors.Wrap(bldr_plugin.ErrEmptyPluginID, "web_plugin_id")
	}
	if c.GetHandlePluginId() == "" {
		return errors.Wrap(bldr_plugin.ErrEmptyPluginID, "handle_plugin_id")
	}
	if err := c.ToForwardConfig().Validate(); err != nil {
		return err
	}
	return nil
}

// ToRequest converts the config into a web plugin request.
func (c *Config) ToRequest() *bldr_web_plugin.HandleRpcViaPluginRequest {
	return &bldr_web_plugin.HandleRpcViaPluginRequest{
		HandlePluginId: c.GetHandlePluginId(),
		ServiceIdRe:    c.GetServiceIdRe(),
		ServerIdRe:     c.GetServerIdRe(),
		Backoff:        c.GetBackoff().CloneVT(),
	}
}

// ToForwardConfig converts the config into a plugin forward-rpc-service config.
func (c *Config) ToForwardConfig() *handle_rpc_viaplugin.Config {
	return &handle_rpc_viaplugin.Config{
		PluginId:    c.GetHandlePluginId(),
		ServiceIdRe: c.GetServiceIdRe(),
		ServerIdRe:  c.GetServerIdRe(),
		Backoff:     c.GetBackoff().CloneVT(),
	}
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
