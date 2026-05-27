//go:build js

package volume_opfs

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
)

// ConfigID is the id attached to the config objects.
var ConfigID = ControllerID

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.GetRootPath() == "" {
		return errors.New("root_path required")
	}
	if version := c.GetStorageFormatVersion(); version != 0 && version != currentStorageFormatVersion {
		return errors.Errorf("unsupported storage_format_version %d", version)
	}
	switch runtimeDriverMode(c) {
	case driverModeAuto, driverModeStandardWasm, driverModeTinyGo:
	default:
		return errors.Errorf("unsupported driver_mode %q", c.GetDriverMode())
	}
	if policy := runtimeResetPolicy(c); policy != resetPolicyAutomatic {
		return errors.Errorf("unsupported reset_policy %q", policy)
	}
	return nil
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ControllerID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig[*Config](c, other)
}

// _ is a type assertion.
var _ config.Config = ((*Config)(nil))
