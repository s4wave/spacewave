package plugin_host_process

import (
	"path/filepath"

	plugin_host_controller "github.com/aperturerobotics/bldr/plugin/host/controller"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
)

// ControllerID is the process host controller ID.
const ControllerID = "bldr/plugin/host/process"

// ConfigID is the config identifier.
const ConfigID = ControllerID

// NewConfig constructs a new controller config.
// Sets the most important fields only.
func NewConfig(
	hostConfig *plugin_host_controller.Config,
	stateDir,
	distDir string,
) *Config {
	return &Config{
		HostConfig: hostConfig,
		StateDir:   stateDir,
		DistDir:    distDir,
	}
}

// GetConfigID returns the unique string for this configuration type.
// This string is stored with the encoded config.
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

// Validate validates the configuration.
// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	if err := c.GetHostConfig().Validate(); err != nil {
		return err
	}
	if !filepath.IsAbs(c.GetStateDir()) {
		return errors.New("state dir: must be absolute path")
	}
	if !filepath.IsAbs(c.GetDistDir()) {
		return errors.New("dist dir: must be absolute path")
	}
	return nil
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
