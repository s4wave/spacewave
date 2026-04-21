package bldr_project_watcher

import (
	"github.com/aperturerobotics/controllerbus/config"
	bldr_project_controller "github.com/s4wave/spacewave/bldr/project/controller"
)

// ConfigID is the identifier for the config type.
const ConfigID = "bldr/project/watcher"

// NewConfig constructs the configuration.
func NewConfig(
	configPath string,
	projCtrlConfig *bldr_project_controller.Config,
) *Config {
	return &Config{
		ConfigPath:              configPath,
		ProjectControllerConfig: projCtrlConfig,
	}
}

// GetConfigID returns the config identifier.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks equality between two configs.
func (c *Config) EqualsConfig(c2 config.Config) bool {
	oc, ok := c2.(*Config)
	if !ok {
		return false
	}

	return c.EqualVT(oc)
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if err := c.GetProjectControllerConfig().Validate(); err != nil {
		return err
	}
	return nil
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
