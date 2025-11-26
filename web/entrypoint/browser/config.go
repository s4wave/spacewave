//go:build js

package web_entrypoint_browser

import (
	"errors"

	web_runtime "github.com/aperturerobotics/bldr/web/runtime"
	"github.com/aperturerobotics/controllerbus/config"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// Validate validates the configuration.
func (c *Config) Validate() error {
	if v := c.GetWebRuntimeId(); v == "" {
		return errors.New("web runtime id must be set")
	}
	if v := c.GetMessagePort(); v == "" {
		return errors.New("message port must be set")
	}

	return nil
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the other config is equal.
func (c *Config) EqualsConfig(other config.Config) bool {
	ot, ok := other.(*Config)
	if !ok {
		return false
	}
	return c.EqualVT(ot)
}

// _ is a type assertion
var _ web_runtime.WebRuntimeConfig = ((*Config)(nil))
