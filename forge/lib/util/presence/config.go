package forge_lib_util_presence

import (
	"path/filepath"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// Validate requires an absolute path and two distinct output names.
func (c *Config) Validate() error {
	if !filepath.IsAbs(c.GetPath()) {
		return errors.Errorf("path must be absolute: %q", c.GetPath())
	}
	if c.GetPresentOutput() == "" || c.GetAbsentOutput() == "" {
		return errors.New("present_output and absent_output must be set")
	}
	if c.GetPresentOutput() == c.GetAbsentOutput() {
		return errors.New("present_output and absent_output must differ")
	}
	return nil
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the other config is equal.
func (c *Config) EqualsConfig(other config.Config) bool {
	oc, ok := other.(*Config)
	return ok && c.EqualVT(oc)
}

// MarshalBlock marshals the block to binary.
func (c *Config) MarshalBlock() ([]byte, error) {
	return c.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (c *Config) UnmarshalBlock(data []byte) error {
	return c.UnmarshalVT(data)
}

// _ is a type assertion
var (
	_ config.Config = (*Config)(nil)
	_ block.Block   = (*Config)(nil)
)
