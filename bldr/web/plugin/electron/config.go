package electron

import (
	web_runtime "github.com/s4wave/spacewave/bldr/web/runtime"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.GetElectronPath() == "" {
		return errors.New("electron path must be set")
	}
	if c.GetRendererPath() == "" {
		return errors.New("renderer path must be set")
	}
	if err := web_runtime.ValidateRuntimeId(c.GetWebRuntimeId()); err != nil {
		return errors.Wrap(err, "web_runtime_id")
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
